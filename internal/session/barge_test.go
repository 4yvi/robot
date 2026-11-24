package session

import (
	"testing"
	"time"
)

func level(amp int16) []int16 {
	c := make([]int16, 1600)
	for i := range c {
		if i%2 == 0 {
			c[i] = amp
		} else {
			c[i] = -amp
		}
	}
	return c
}

func TestBargeGateRejectsEchoAcceptsLoudUser(t *testing.T) {
	g := newBargeGate()
	g.started = time.Now().Add(-time.Second) // warm-up over
	for i := 0; i < 30; i++ {
		g.feed(level(1500)) // the assistant's own voice at the mic
	}
	if g.feed(level(1600)) {
		t.Fatal("echo-level audio must not count as the user")
	}
	var ok bool
	for i := 0; i < 3; i++ {
		ok = g.feed(level(6000)) // user speaking close to the mic
	}
	if !ok {
		t.Fatal("clearly louder speech must be accepted")
	}
}

func TestBargeGateWarmup(t *testing.T) {
	g := newBargeGate()
	if g.feed(level(20000)) {
		t.Fatal("detections right after playback starts must be ignored")
	}
}
