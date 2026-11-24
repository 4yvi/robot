package session

import (
	"time"

	"github.com/4yvi/robot/internal/audio"
)

// bargeGate decides whether a wake word heard while the assistant is talking
// is the user or just the assistant's own voice coming back through the
// microphone. It tracks the level of that echo and accepts a detection only
// when the recent peak is clearly louder than the echo.
type bargeGate struct {
	started time.Time
	echo    float64 // running average of the level while we talk
	peak    float64 // decaying peak of recent chunks
}

const (
	bargeWarmup   = 800 * time.Millisecond // ignore the first moments: echo level unknown
	bargeFactor   = 1.8                    // how much louder than the echo the user must be
	bargeMinLevel = 800.0                  // absolute floor (RMS of int16 samples)
)

func newBargeGate() *bargeGate { return &bargeGate{started: time.Now()} }

// feed updates the levels with one chunk and reports whether a detection right
// now should be believed.
func (g *bargeGate) feed(c []int16) bool {
	r := audio.RMS(c)
	if g.echo == 0 || r < g.echo*1.5 { // quiet or echo-like: learn the echo level
		if g.echo == 0 {
			g.echo = r
		} else {
			g.echo += 0.05 * (r - g.echo)
		}
	}
	g.peak = max(r, g.peak*0.97)
	if time.Since(g.started) < bargeWarmup {
		return false
	}
	return g.peak >= max(bargeMinLevel, g.echo*bargeFactor)
}
