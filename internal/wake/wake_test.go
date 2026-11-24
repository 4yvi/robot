package wake

import (
	"context"
	"testing"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		text, word string
		want       bool
	}{
		{"робот", "робот", true},
		{"Привет РОБОТ как дела", "робот", true},
		{"роботы", "робот", false},
		{"", "робот", false},
	}
	for _, c := range cases {
		if Match(c.text, c.word) != c.want {
			t.Errorf("Match(%q,%q) != %v", c.text, c.word, c.want)
		}
	}
	if !Match("ёж", "еж") {
		t.Error("ё should fold to е")
	}
}

func TestGrammar(t *testing.T) {
	if g := Grammar("Робот"); g != `["робот","[unk]"]` {
		t.Fatalf("grammar = %s", g)
	}
}

func TestTextFromJSON(t *testing.T) {
	if textFromJSON(`{"partial":"робот"}`) != "робот" || textFromJSON(`{"text":"привет"}`) != "привет" {
		t.Fatal("bad parse")
	}
}

type fake struct{ n int }

func (f *fake) Accept([]int16) bool { f.n++; return f.n == 2 }
func (f *fake) Reset()              {}
func (f *fake) Close()              {}

func TestListen(t *testing.T) {
	ch := make(chan []int16, 3)
	for i := 0; i < 3; i++ {
		ch <- []int16{1}
	}
	close(ch)
	hits := 0
	Listen(context.Background(), &fake{}, ch, func() { hits++ })
	if hits != 1 {
		t.Fatalf("hits=%d", hits)
	}
}

func TestGateRejectsLongSpeech(t *testing.T) {
	loud := make([]int16, 1600)
	for i := range loud {
		loud[i] = 5000
	}
	g := NewGate()
	for i := 0; i < 8; i++ { // 800 ms
		g.Feed(loud)
	}
	if !g.ShortUtterance() {
		t.Fatal("800 ms should be short")
	}
	g.Reset()
	for i := 0; i < 30; i++ { // 3 s
		g.Feed(loud)
	}
	if g.ShortUtterance() {
		t.Fatal("3 s should be rejected")
	}
}
