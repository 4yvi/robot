// Package wake detects the wake word in a stream of audio.
package wake

import (
	"context"
	"encoding/json"
	"strings"
)

// Detector consumes 16 kHz mono PCM and reports when the wake word was heard.
type Detector interface {
	// Accept feeds a chunk and returns true when the wake word is detected.
	// After a detection the detector resets itself.
	Accept(pcm []int16) bool
	// Reset drops any buffered state.
	Reset()
	Close()
}

// Listen feeds chunks to d until ctx ends or the channel closes, calling
// onWake for every detection. It returns when the stream ends.
func Listen(ctx context.Context, d Detector, chunks <-chan []int16, onWake func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-chunks:
			if !ok {
				return
			}
			if d.Accept(c) {
				onWake()
			}
		}
	}
}

// Normalize lowercases and folds ё to е.
func Normalize(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "ё", "е")
}

// Match reports whether text contains word as a whole word.
func Match(text, word string) bool {
	word = Normalize(word)
	if word == "" {
		return false
	}
	for _, f := range strings.Fields(Normalize(text)) {
		if f == word {
			return true
		}
	}
	return false
}

// textFromJSON extracts "text" or "partial" from a Vosk result.
func textFromJSON(raw string) string {
	var r struct {
		Text    string `json:"text"`
		Partial string `json:"partial"`
	}
	if json.Unmarshal([]byte(raw), &r) != nil {
		return ""
	}
	if r.Text != "" {
		return r.Text
	}
	return r.Partial
}

// Grammar builds the restricted Vosk grammar: the wake word and [unk].
func Grammar(word string) string {
	b, _ := json.Marshal([]string{Normalize(word), "[unk]"})
	return string(b)
}

// Gate measures how long the current continuous stretch of speech is.
// Vosk with a tiny grammar tends to map any sentence onto the wake word, but
// a real wake word is a short utterance, so long stretches are rejected.
type Gate struct {
	Threshold float64 // RMS that counts as speech
	GapMS     int     // silence that separates utterances
	MaxMS     int     // longest utterance that may be a wake word

	nowMS, startMS, lastEndMS, silentMS int
	inSpeech                            bool
}

// NewGate returns a gate with sensible defaults.
func NewGate() *Gate { return &Gate{Threshold: 300, GapMS: 500, MaxMS: 1800} }

// Feed advances the gate by one chunk of 16 kHz samples.
func (g *Gate) Feed(pcm []int16) {
	ms := len(pcm) * 1000 / 16000
	var sum float64
	for _, v := range pcm {
		sum += float64(v) * float64(v)
	}
	loud := len(pcm) > 0 && sum/float64(len(pcm)) >= g.Threshold*g.Threshold
	if loud {
		if !g.inSpeech {
			g.startMS = g.nowMS
			g.inSpeech = true
		}
		g.silentMS = 0
		g.lastEndMS = g.nowMS + ms
	} else if g.inSpeech {
		g.silentMS += ms
		if g.silentMS >= g.GapMS {
			g.inSpeech = false
		}
	}
	g.nowMS += ms
}

// ShortUtterance reports whether the latest speech stretch is short enough.
func (g *Gate) ShortUtterance() bool {
	return g.lastEndMS > g.startMS && g.lastEndMS-g.startMS <= g.MaxMS
}

// Reset clears the gate.
func (g *Gate) Reset() { *g = Gate{Threshold: g.Threshold, GapMS: g.GapMS, MaxMS: g.MaxMS} }
