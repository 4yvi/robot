//go:build vosk

package wake

import (
	"encoding/binary"
	"fmt"

	vosk "github.com/alphacep/vosk-api/go"
)

// Vosk is a Detector backed by a Vosk recognizer with a restricted grammar.
type Vosk struct {
	model *vosk.VoskModel
	rec   *vosk.VoskRecognizer
	word  string
	gate  *Gate
}

// NewVosk loads the model and creates a recognizer limited to the wake word and [unk].
func NewVosk(modelPath, word string, sampleRate int) (Detector, error) {
	vosk.SetLogLevel(-1)
	m, err := vosk.NewModel(modelPath)
	if err != nil {
		return nil, fmt.Errorf("load vosk model %q: %w", modelPath, err)
	}
	r, err := vosk.NewRecognizerGrm(m, float64(sampleRate), []byte(Grammar(word)))
	if err != nil {
		m.Free()
		return nil, fmt.Errorf("create vosk recognizer: %w", err)
	}
	return &Vosk{model: m, rec: r, word: word, gate: NewGate()}, nil
}

// Accept implements Detector. Only final results count (partials fire on the
// start of any sentence), and the utterance must be short (see Gate).
func (v *Vosk) Accept(pcm []int16) bool {
	v.gate.Feed(pcm)
	buf := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(s))
	}
	if v.rec.AcceptWaveform(buf) == 0 {
		return false
	}
	text := textFromJSON(string(v.rec.Result()))
	hit := Match(text, v.word) && v.gate.ShortUtterance()
	v.gate.Reset()
	return hit
}

// Reset implements Detector.
func (v *Vosk) Reset() { v.rec.Reset(); v.gate.Reset() }

// Close implements Detector.
func (v *Vosk) Close() {
	v.rec.Free()
	v.model.Free()
}
