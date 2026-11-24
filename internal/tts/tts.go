// Package tts synthesizes speech with Piper and plays it sentence by sentence.
package tts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/4yvi/robot/internal/audio"
)

// Synthesizer converts text to WAV.
type Synthesizer interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

// Piper runs the piper executable once per sentence.
type Piper struct {
	Path  string // piper executable
	Voice string // .onnx voice model
}

// Synthesize pipes text to piper's stdin and returns the WAV it writes.
func (p *Piper) Synthesize(ctx context.Context, text string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "robot-tts-*.wav")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(name)

	cmd := exec.CommandContext(ctx, p.Path, "--model", p.Voice, "--output_file", name)
	cmd.Dir = filepath.Dir(p.Path) // piper finds espeak-ng-data next to itself
	cmd.Stdin = strings.NewReader(strings.TrimSpace(text) + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("piper: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(name)
}

// Speaker synthesizes and plays sentences, preparing the next one while the
// current one plays so the first sentence is heard as early as possible.
type Speaker struct {
	Synth  Synthesizer
	Player audio.Player
}

// Speak says text. It returns when done or when ctx is cancelled (interrupt).
func (s *Speaker) Speak(ctx context.Context, text string) error {
	ch := make(chan string, 1)
	ch <- text
	close(ch)
	return s.SpeakStream(ctx, ch)
}

// SpeakStream speaks text arriving in pieces (e.g. LLM tokens), starting as
// soon as the first sentence is complete.
func (s *Speaker) SpeakStream(ctx context.Context, pieces <-chan string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sentences := make(chan string, 8)
	go func() {
		defer close(sentences)
		var buf SentenceBuffer
		emit := func(list []string) bool {
			for _, sent := range list {
				select {
				case sentences <- sent:
				case <-ctx.Done():
					return false
				}
			}
			return true
		}
		for {
			select {
			case <-ctx.Done():
				return
			case p, ok := <-pieces:
				if !ok {
					emit(buf.Flush())
					return
				}
				if !emit(buf.Push(p)) {
					return
				}
			}
		}
	}()

	type clip struct {
		wav []byte
		err error
	}
	clips := make(chan clip, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(clips)
		for sent := range sentences {
			wav, err := s.Synth.Synthesize(ctx, sent)
			select {
			case clips <- clip{wav, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var firstErr error
	for c := range clips {
		if c.err != nil {
			firstErr = c.err
			break
		}
		if err := s.Player.Play(ctx, c.wav); err != nil {
			firstErr = err
			break
		}
	}
	cancel()
	for range clips { // drain so the producer can exit
	}
	wg.Wait()
	if firstErr == nil && ctx.Err() != nil {
		return nil
	}
	return firstErr
}
