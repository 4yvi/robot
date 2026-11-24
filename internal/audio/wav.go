package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// EncodeWAV wraps 16-bit PCM samples into a WAV container.
func EncodeWAV(samples []int16, sampleRate, channels int) []byte {
	dataLen := len(samples) * 2
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+dataLen))
	b.WriteString("WAVEfmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate*channels*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(dataLen))
	_ = binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

// DecodeWAV parses a 16-bit PCM WAV file.
func DecodeWAV(wav []byte) (samples []int16, sampleRate, channels int, err error) {
	if len(wav) < 12 || string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, 0, 0, errors.New("not a WAV file")
	}
	pos := 12
	bits := 0
	for pos+8 <= len(wav) {
		id := string(wav[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(wav[pos+4 : pos+8]))
		body := pos + 8
		end := body + size
		if end > len(wav) || size < 0 {
			end = len(wav) // streaming WAVs (e.g. from Piper) may carry a bogus size
		}
		switch id {
		case "fmt ":
			if end-body < 16 {
				return nil, 0, 0, errors.New("short fmt chunk")
			}
			if binary.LittleEndian.Uint16(wav[body:]) != 1 {
				return nil, 0, 0, errors.New("only PCM WAV is supported")
			}
			channels = int(binary.LittleEndian.Uint16(wav[body+2:]))
			sampleRate = int(binary.LittleEndian.Uint32(wav[body+4:]))
			bits = int(binary.LittleEndian.Uint16(wav[body+14:]))
		case "data":
			if bits != 16 {
				return nil, 0, 0, errors.New("only 16-bit WAV is supported")
			}
			raw := wav[body:end]
			samples = make([]int16, len(raw)/2)
			for i := range samples {
				samples[i] = int16(binary.LittleEndian.Uint16(raw[i*2:]))
			}
			return samples, sampleRate, channels, nil
		}
		pos = end + (size & 1)
	}
	return nil, 0, 0, errors.New("no data chunk")
}

// Beep generates a short sine beep as WAV (used when the wake word fires).
func Beep(freq float64, ms int) []byte {
	n := SampleRate * ms / 1000
	s := make([]int16, n)
	for i := range s {
		env := 1.0
		if edge := n / 10; i < edge {
			env = float64(i) / float64(edge)
		} else if i > n-edge {
			env = float64(n-i) / float64(edge)
		}
		s[i] = int16(9000 * env * sin(2*3.141592653589793*freq*float64(i)/SampleRate))
	}
	return EncodeWAV(s, SampleRate, 1)
}
