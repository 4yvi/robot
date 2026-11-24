package tts

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSplit(t *testing.T) {
	got := Split("Привет, как дела? Всё хорошо. Да!")
	want := []string{"Привет, как дела?", "Всё хорошо. Да!"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestSplitKeepsDecimals(t *testing.T) {
	got := Split("Версия 3.5 вышла вчера. Это хорошо.")
	if len(got) != 2 || !strings.HasPrefix(got[0], "Версия 3.5") {
		t.Fatalf("got %q", got)
	}
}

func TestSplitLong(t *testing.T) {
	long := strings.Repeat("слово ", 100)
	for _, p := range Split(long) {
		if len([]rune(p)) > maxSentence {
			t.Fatalf("piece too long: %d", len([]rune(p)))
		}
	}
}

func TestSentenceBufferStreaming(t *testing.T) {
	var b SentenceBuffer
	var got []string
	for _, tok := range []string{"Это пер", "вое предложение. Вто", "рое предложение.", " Хвост"} {
		got = append(got, b.Push(tok)...)
	}
	got = append(got, b.Flush()...)
	want := []string{"Это первое предложение.", "Второе предложение.", "Хвост"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

type fakeSynth struct{}

func (fakeSynth) Synthesize(_ context.Context, text string) ([]byte, error) { return []byte(text), nil }

type fakePlayer struct {
	mu    sync.Mutex
	heard []string
	delay time.Duration
}

func (p *fakePlayer) Play(ctx context.Context, wav []byte) error {
	select {
	case <-time.After(p.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	p.mu.Lock()
	p.heard = append(p.heard, string(wav))
	p.mu.Unlock()
	return nil
}

func TestSpeakerPlaysInOrder(t *testing.T) {
	p := &fakePlayer{}
	s := &Speaker{Synth: fakeSynth{}, Player: p}
	if err := s.Speak(context.Background(), "Первое предложение здесь. Второе предложение здесь."); err != nil {
		t.Fatal(err)
	}
	if len(p.heard) != 2 || p.heard[0] != "Первое предложение здесь." {
		t.Fatalf("heard %q", p.heard)
	}
}

func TestSpeakerInterrupt(t *testing.T) {
	p := &fakePlayer{delay: 500 * time.Millisecond}
	s := &Speaker{Synth: fakeSynth{}, Player: p}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_ = s.Speak(ctx, "Первое предложение здесь. Второе предложение здесь.")
	if time.Since(start) > 300*time.Millisecond || len(p.heard) != 0 {
		t.Fatalf("not interrupted: %v heard=%q", time.Since(start), p.heard)
	}
}
