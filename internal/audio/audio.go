// Package audio provides microphone capture, WAV playback and silence detection.
package audio

import "context"

const (
	SampleRate = 16000
	Channels   = 1
)

// Device describes an audio device.
type Device struct {
	Name      string
	IsDefault bool
}

// Recorder captures 16 kHz mono 16-bit audio.
type Recorder interface {
	// Start begins capture and delivers PCM chunks until ctx is cancelled.
	Start(ctx context.Context) (<-chan []int16, error)
}

// Player plays WAV data on the output device.
type Player interface {
	// Play blocks until playback ends or ctx is cancelled.
	Play(ctx context.Context, wav []byte) error
}

// Lister lists input and output devices.
type Lister interface {
	Capture() ([]Device, error)
	Playback() ([]Device, error)
}
