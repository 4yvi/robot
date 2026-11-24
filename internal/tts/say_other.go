//go:build !darwin

package tts

import (
	"context"
	"errors"
)

// Say is only available on macOS.
type Say struct{ Voice string }

// Synthesize implements Synthesizer.
func (s *Say) Synthesize(context.Context, string) ([]byte, error) {
	return nil, errors.New("the say engine exists only on macOS; use tts_engine: piper")
}
