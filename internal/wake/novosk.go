//go:build !vosk

package wake

import "errors"

// ErrNoVosk is returned by builds made without the `vosk` tag.
var ErrNoVosk = errors.New("this build has no Vosk support; rebuild with -tags vosk (see README)")

// NewVosk is unavailable without the vosk build tag.
func NewVosk(modelPath, word string, sampleRate int) (Detector, error) {
	return nil, ErrNoVosk
}
