//go:build darwin

package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Say synthesizes speech with the macOS `say` command.
type Say struct{ Voice string }

// Synthesize implements Synthesizer; it returns a 22 kHz 16-bit WAV.
func (s *Say) Synthesize(ctx context.Context, text string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "robot-say-*.wav")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(name)
	voice := s.Voice
	if voice == "" {
		voice = "Milena"
	}
	cmd := exec.CommandContext(ctx, "say", "-v", voice, "-o", name, "--data-format=LEI16@22050", "--", strings.TrimSpace(text))
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("say: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(name)
}
