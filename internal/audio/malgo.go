package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// Malgo implements Recorder, Player and Lister on top of miniaudio.
type Malgo struct {
	ctx           *malgo.AllocatedContext
	MicDevice     string // name or substring; empty = default
	SpeakerDevice string
}

// New initialises the audio backend.
func New(mic, speaker string) (*Malgo, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio: %w", err)
	}
	return &Malgo{ctx: ctx, MicDevice: mic, SpeakerDevice: speaker}, nil
}

// Close releases the backend.
func (m *Malgo) Close() {
	_ = m.ctx.Uninit()
	m.ctx.Free()
}

func (m *Malgo) list(kind malgo.DeviceType) ([]Device, error) {
	infos, err := m.ctx.Devices(kind)
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(infos))
	for i := range infos {
		out = append(out, Device{Name: infos[i].Name(), IsDefault: infos[i].IsDefault != 0})
	}
	return out, nil
}

// Capture lists input devices.
func (m *Malgo) Capture() ([]Device, error) { return m.list(malgo.Capture) }

// Playback lists output devices.
func (m *Malgo) Playback() ([]Device, error) { return m.list(malgo.Playback) }

// find returns a pointer to the device ID matching name, or nil for default.
func (m *Malgo) find(kind malgo.DeviceType, name string) (unsafe.Pointer, error) {
	if name == "" {
		return nil, nil
	}
	infos, err := m.ctx.Devices(kind)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(name)
	for i := range infos {
		if strings.Contains(strings.ToLower(infos[i].Name()), want) {
			return infos[i].ID.Pointer(), nil
		}
	}
	return nil, fmt.Errorf("audio device %q not found", name)
}

// SetMic changes the capture device used by the next Start.
func (m *Malgo) SetMic(name string) { m.MicDevice = name }

// Start captures 16 kHz mono s16 audio until ctx is cancelled.
func (m *Malgo) Start(ctx context.Context) (<-chan []int16, error) {
	id, err := m.find(malgo.Capture, m.MicDevice)
	if err != nil {
		return nil, err
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = Channels
	cfg.SampleRate = SampleRate
	cfg.Capture.DeviceID = id

	ch := make(chan []int16, 64)
	var once sync.Once
	closed := make(chan struct{})
	cb := malgo.DeviceCallbacks{Data: func(_, in []byte, frames uint32) {
		s := make([]int16, len(in)/2)
		for i := range s {
			s[i] = int16(binary.LittleEndian.Uint16(in[i*2:]))
		}
		select {
		case ch <- s:
		case <-closed:
		default: // consumer too slow: drop rather than block the audio thread
		}
	}}
	dev, err := malgo.InitDevice(m.ctx.Context, cfg, cb)
	if err != nil {
		return nil, fmt.Errorf("open microphone: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return nil, fmt.Errorf("start microphone: %w", err)
	}
	go func() {
		<-ctx.Done()
		once.Do(func() { close(closed) })
		dev.Uninit()
		close(ch)
	}()
	return ch, nil
}

// Play plays a 16-bit PCM WAV and blocks until it ends or ctx is cancelled.
func (m *Malgo) Play(ctx context.Context, wav []byte) error {
	samples, rate, channels, err := DecodeWAV(wav)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}
	id, err := m.find(malgo.Playback, m.SpeakerDevice)
	if err != nil {
		return err
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = uint32(channels)
	cfg.SampleRate = uint32(rate)
	cfg.Playback.DeviceID = id

	var mu sync.Mutex
	pos := 0
	done := make(chan struct{})
	var once sync.Once
	cb := malgo.DeviceCallbacks{Data: func(out, _ []byte, _ uint32) {
		mu.Lock()
		defer mu.Unlock()
		for i := 0; i+1 < len(out); i += 2 {
			var v int16
			if pos < len(samples) {
				v = samples[pos]
				pos++
			}
			binary.LittleEndian.PutUint16(out[i:], uint16(v))
		}
		if pos >= len(samples) {
			once.Do(func() { close(done) })
		}
	}}
	dev, err := malgo.InitDevice(m.ctx.Context, cfg, cb)
	if err != nil {
		return fmt.Errorf("open speaker: %w", err)
	}
	defer dev.Uninit()
	if err := dev.Start(); err != nil {
		return err
	}
	select {
	case <-done:
		// let the device buffer drain
		select {
		case <-ctx.Done():
		case <-after(150):
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
