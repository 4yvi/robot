package audio

import "math"

func sin(x float64) float64 { return math.Sin(x) }

// RMS returns the root-mean-square energy of the samples.
func RMS(s []int16) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(s)))
}

// VAD is an energy-based voice activity / silence detector.
type VAD struct {
	Threshold   float64 // RMS above this counts as speech
	SilenceMS   int     // trailing silence that ends a phrase
	MaxMS       int     // hard limit for a phrase
	heardSpeech bool
	silentMS    int
	totalMS     int
}

// NewVAD creates a detector with a default threshold.
func NewVAD(silenceMS, maxMS int) *VAD {
	return &VAD{Threshold: 500, SilenceMS: silenceMS, MaxMS: maxMS}
}

// Feed processes one chunk and reports whether the phrase is finished.
func (v *VAD) Feed(chunk []int16) (done bool) {
	ms := len(chunk) * 1000 / SampleRate
	v.totalMS += ms
	if RMS(chunk) >= v.Threshold {
		v.heardSpeech = true
		v.silentMS = 0
	} else {
		v.silentMS += ms
	}
	if v.MaxMS > 0 && v.totalMS >= v.MaxMS {
		return true
	}
	return v.heardSpeech && v.silentMS >= v.SilenceMS
}

// HeardSpeech reports whether any speech has been detected yet.
func (v *VAD) HeardSpeech() bool { return v.heardSpeech }

// TotalMS is the amount of audio fed so far.
func (v *VAD) TotalMS() int { return v.totalMS }

// Reset clears the state.
func (v *VAD) Reset() { v.heardSpeech, v.silentMS, v.totalMS = false, 0, 0 }
