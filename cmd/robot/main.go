// Command robot is a local voice assistant.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/4yvi/robot/internal/audio"
	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/stt"
	"github.com/4yvi/robot/internal/wake"
)

var version = "dev"

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfgPath, err := config.FindPath()
	fatal(err)
	cfg, err := config.Load(cfgPath)
	fatal(err)
	closeLog := setupLogging(cmd != "")
	defer closeLog()

	switch cmd {
	case "version", "--version":
		fmt.Println("robot", version)
	case "devices":
		cmdDevices(cfg)
	case "listen":
		cmdListen(cfg)
	case "record-test":
		cmdRecordTest(cfg)
	default:
		runCommand(cmd, cfg)
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// setupLogging writes logs to <config dir>/logs/robot.log (and stderr for CLI commands).
func setupLogging(alsoStderr bool) func() {
	dir, err := config.Dir()
	if err != nil {
		return func() {}
	}
	logDir := filepath.Join(dir, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	f, err := os.OpenFile(filepath.Join(logDir, "robot.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}
	}
	var w io.Writer = f
	if alsoStderr {
		w = io.MultiWriter(f, os.Stderr)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return func() { _ = f.Close() }
}

func cmdDevices(cfg *config.Config) {
	a, err := audio.New(cfg.MicDevice, cfg.SpeakerDevice)
	fatal(err)
	defer a.Close()
	print := func(title string, list []Device, err error) {
		fatal(err)
		fmt.Println(title)
		for _, d := range list {
			mark := " "
			if d.IsDefault {
				mark = "*"
			}
			fmt.Printf("  %s %s\n", mark, d.Name)
		}
	}
	in, err := a.Capture()
	print("Input devices (* = default):", in, err)
	out, err := a.Playback()
	print("Output devices (* = default):", out, err)
}

// Device aliases audio.Device for brevity.
type Device = audio.Device

func cmdRecordTest(cfg *config.Config) {
	a, err := audio.New(cfg.MicDevice, cfg.SpeakerDevice)
	fatal(err)
	defer a.Close()
	fmt.Println("Recording 5 seconds, speak now...")
	samples, err := audio.Record(context.Background(), a, 5*time.Second)
	fatal(err)
	fmt.Printf("Captured %d samples (peak RMS %.0f). Playing back...\n", len(samples), audio.RMS(samples))
	fatal(a.Play(context.Background(), audio.EncodeWAV(samples, audio.SampleRate, audio.Channels)))
}

// cmdListen runs the wake-word loop and beeps on every detection (stage 2 check).
func cmdListen(cfg *config.Config) {
	a, err := audio.New(cfg.MicDevice, cfg.SpeakerDevice)
	fatal(err)
	defer a.Close()
	det, err := wake.NewVosk(cfg.ResolvePath(cfg.VoskModelPath), cfg.WakeWord, audio.SampleRate)
	fatal(err)
	defer det.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	chunks, err := a.Start(ctx)
	fatal(err)
	fmt.Printf("Listening for %q, Ctrl+C to quit\n", cfg.WakeWord)
	beep := audio.Beep(880, 150)
	tr := stt.New(cfg.WhisperURL, "ru")
	wake.Listen(ctx, det, chunks, func() {
		slog.Info("wake word detected", "word", cfg.WakeWord)
		fmt.Println("wake! speak now...")
		_ = a.Play(ctx, beep)
		vad := audio.NewVAD(cfg.SilenceMS, cfg.MaxPhraseSeconds*1000)
		pcm, err := audio.RecordPhrase(ctx, chunks, vad, 5000)
		if err != nil {
			slog.Warn("record phrase", "err", err)
			return
		}
		text, err := tr.Transcribe(ctx, audio.EncodeWAV(pcm, audio.SampleRate, audio.Channels))
		if err != nil {
			slog.Error("transcribe", "err", err)
			return
		}
		slog.Info("recognized", "text", text)
		fmt.Println("you said:", text)
		det.Reset()
	})
}
