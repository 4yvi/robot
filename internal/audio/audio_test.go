package audio

import (
	"context"
	"testing"
)

func tone(n int, amp float64) []int16 {
	s := make([]int16, n)
	for i := range s {
		s[i] = int16(amp * sin(float64(i)*0.3))
	}
	return s
}

func TestWAVRoundTrip(t *testing.T) {
	in := tone(1600, 8000)
	out, rate, ch, err := DecodeWAV(EncodeWAV(in, 16000, 1))
	if err != nil || rate != 16000 || ch != 1 || len(out) != len(in) {
		t.Fatalf("err=%v rate=%d ch=%d len=%d", err, rate, ch, len(out))
	}
	for i := range in {
		if in[i] != out[i] {
			t.Fatalf("sample %d differs", i)
		}
	}
}

func TestVADEndsAfterSilence(t *testing.T) {
	v := NewVAD(1200, 30000)
	chunk := 1600 // 100 ms
	for i := 0; i < 5; i++ {
		if v.Feed(tone(chunk, 8000)) {
			t.Fatal("ended during speech")
		}
	}
	done := false
	for i := 0; i < 12; i++ {
		done = v.Feed(make([]int16, chunk))
	}
	if !done {
		t.Fatal("expected end after 1200 ms of silence")
	}
}

func TestVADSilenceOnlyDoesNotFinish(t *testing.T) {
	v := NewVAD(1200, 30000)
	for i := 0; i < 20; i++ {
		if v.Feed(make([]int16, 1600)) {
			t.Fatal("finished without speech")
		}
	}
}

func TestVADMaxLength(t *testing.T) {
	v := NewVAD(1200, 1000)
	done := false
	for i := 0; i < 10; i++ {
		done = v.Feed(tone(1600, 8000))
	}
	if !done {
		t.Fatal("max length not enforced")
	}
}

func TestRecordPhrase(t *testing.T) {
	ch := make(chan []int16, 40)
	for i := 0; i < 3; i++ {
		ch <- tone(1600, 8000)
	}
	for i := 0; i < 15; i++ {
		ch <- make([]int16, 1600)
	}
	got, err := RecordPhrase(context.Background(), ch, NewVAD(1200, 30000), 0)
	if err != nil || len(got) != 1600*15 {
		t.Fatalf("err=%v len=%d", err, len(got))
	}
}

func TestRecordPhraseNoSpeech(t *testing.T) {
	ch := make(chan []int16, 40)
	for i := 0; i < 30; i++ {
		ch <- make([]int16, 1600)
	}
	_, err := RecordPhrase(context.Background(), ch, NewVAD(1200, 30000), 2000)
	if err != ErrNoSpeech {
		t.Fatalf("err=%v", err)
	}
}
