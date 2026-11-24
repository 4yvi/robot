package tts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sherpa synthesizes speech with the sherpa-onnx offline TTS command line
// tool and a Piper VITS voice (neural, much more natural than system voices).
type Sherpa struct {
	Exe      string  // sherpa-onnx-offline-tts
	VoiceDir string  // folder with <name>.onnx, tokens.txt and espeak-ng-data
	Speed    float64 // 1.0 = normal
}

// voiceFiles locates the model and tokens in VoiceDir.
func (s *Sherpa) voiceFiles() (model, tokens, data string, err error) {
	matches, _ := filepath.Glob(filepath.Join(s.VoiceDir, "*.onnx"))
	if len(matches) == 0 {
		return "", "", "", fmt.Errorf("no .onnx voice model in %s", s.VoiceDir)
	}
	return matches[0], filepath.Join(s.VoiceDir, "tokens.txt"), filepath.Join(s.VoiceDir, "espeak-ng-data"), nil
}

// Available reports whether the tool and a voice are present.
func (s *Sherpa) Available() bool {
	if _, err := os.Stat(s.Exe); err != nil {
		return false
	}
	_, _, _, err := s.voiceFiles()
	return err == nil
}

// args builds the command line for text.
func (s *Sherpa) args(model, tokens, data, out, text string) []string {
	a := []string{
		"--vits-model=" + model,
		"--vits-tokens=" + tokens,
		"--vits-data-dir=" + data,
		"--output-filename=" + out,
	}
	if s.Speed > 0 && s.Speed != 1 {
		a = append(a, fmt.Sprintf("--vits-length-scale=%.2f", 1/s.Speed))
	}
	return append(a, text)
}

// Synthesize implements Synthesizer.
func (s *Sherpa) Synthesize(ctx context.Context, text string) ([]byte, error) {
	model, tokens, data, err := s.voiceFiles()
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "robot-sherpa-*.wav")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(name)

	cmd := exec.CommandContext(ctx, s.Exe, s.args(model, tokens, data, name, strings.TrimSpace(text))...)
	binDir := filepath.Dir(s.Exe)
	cmd.Dir = binDir
	lib := filepath.Join(binDir, "..", "lib")
	cmd.Env = append(os.Environ(), "DYLD_LIBRARY_PATH="+lib, "LD_LIBRARY_PATH="+lib)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("sherpa-onnx tts: %w: %s", err, lastLines(out.String(), 3))
	}
	return os.ReadFile(name)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
