package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/4yvi/robot/internal/audio"
	"github.com/4yvi/robot/internal/clarify"
	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/llm"
	"github.com/4yvi/robot/internal/memory"
	"github.com/4yvi/robot/internal/platform"
	"github.com/4yvi/robot/internal/session"
	"github.com/4yvi/robot/internal/stt"
	"github.com/4yvi/robot/internal/tools"
	"github.com/4yvi/robot/internal/tts"
	"github.com/4yvi/robot/internal/wake"
)

func init() {
	// the tray (Cocoa/Win32 message loop) must run on the main OS thread
	runtime.LockOSThread()
}

func stdout() io.Writer { return os.Stdout }

func dbPath() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "robot.db"), nil
}

func openStore() (*memory.SQLite, error) {
	p, err := dbPath()
	if err != nil {
		return nil, err
	}
	return memory.Open(p)
}

// runCommand handles `robot`, `robot run`, `robot headless`, `robot history`.
func runCommand(cmd string, args []string, cfg *config.Config) {
	switch cmd {
	case "", "run":
		runApp(cfg, true)
	case "headless":
		runApp(cfg, false)
	case "history":
		cmdHistory(args)
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		fmt.Fprintln(os.Stderr, "commands: run, headless, devices, listen, say, record-test, history, version")
		os.Exit(2)
	}
}

// App wires every component together.
type App struct {
	cfg   *config.Config
	audio *audio.Malgo
	store *memory.SQLite
	llm   *llm.HTTPClient
	mgr   *session.Manager
}

func newApp(cfg *config.Config) (*App, error) {
	a, err := audio.New(cfg.MicDevice, cfg.SpeakerDevice)
	if err != nil {
		return nil, err
	}
	store, err := openStore()
	if err != nil {
		a.Close()
		return nil, err
	}
	lm := llm.New(cfg.LMStudioURL, cfg.ModelName)
	if cfg.ModelName == "" { // use the first model LM Studio reports
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if ids, err := lm.Models(ctx); err == nil && len(ids) > 0 {
			lm.SetModel(ids[0])
			slog.Info("model auto-selected", "model", ids[0])
		}
		cancel()
	}
	env := &tools.Env{Config: cfg, Memory: store}
	mgr, err := session.New(session.Deps{
		Cfg:    cfg,
		Mic:    a,
		Player: a,
		NewDetector: func(word string) (wake.Detector, error) {
			return wake.NewVosk(cfg.ResolvePath(cfg.VoskModelPath), word, audio.SampleRate)
		},
		STT:     stt.New(cfg.WhisperURL, "ru"),
		LLM:     lm,
		Clarify: &clarify.Step{Checker: clarify.LLM{Client: lm}, Max: cfg.MaxClarifications},
		Tools:   tools.NewDefault(env),
		Speaker: &tts.Speaker{
			Synth:  &tts.Piper{Path: cfg.ResolvePath(cfg.PiperPath), Voice: cfg.ResolvePath(cfg.PiperVoice)},
			Player: a,
		},
		Store: store,
	})
	if err != nil {
		store.Close()
		a.Close()
		return nil, err
	}
	env.Say = mgr.Say
	env.Confirm = mgr.Confirm
	return &App{cfg: cfg, audio: a, store: store, llm: lm, mgr: mgr}, nil
}

func (a *App) close() {
	a.mgr.Close()
	a.store.Close()
	a.audio.Close()
}

// runApp starts the assistant, with the tray icon unless withTray is false.
func runApp(cfg *config.Config, withTray bool) {
	release, ok, err := platform.Lock("robot-assistant")
	if err != nil {
		slog.Warn("single-instance lock", "err", err)
	} else if !ok {
		slog.Info("another instance is already running, exiting")
		fmt.Fprintln(os.Stderr, "robot is already running")
		return
	}
	if release != nil {
		defer release()
	}

	app, err := newApp(cfg)
	if err != nil {
		slog.Error("startup failed", "err", err)
		fatal(err)
	}
	defer app.close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	slog.Info("robot started", "version", version, "config", cfg.Path())

	if !withTray {
		_ = app.mgr.Run(ctx)
		return
	}
	go func() {
		if err := app.mgr.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("dialog loop stopped", "err", err)
		}
	}()
	runTray(ctx, stop, app)
}
