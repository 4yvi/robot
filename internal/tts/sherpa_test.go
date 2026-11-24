package tts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSherpaArgsAndAvailability(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin", "sherpa-onnx-offline-tts")
	voice := filepath.Join(dir, "voice")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	_ = os.MkdirAll(voice, 0o755)
	s := &Sherpa{Exe: exe, VoiceDir: voice, Speed: 1.25}
	if s.Available() {
		t.Fatal("nothing installed yet")
	}
	_ = os.WriteFile(exe, nil, 0o755)
	_ = os.WriteFile(filepath.Join(voice, "ru.onnx"), nil, 0o644)
	if !s.Available() {
		t.Fatal("should be available")
	}
	m, tok, data, _ := s.voiceFiles()
	a := strings.Join(s.args(m, tok, data, "o.wav", "Привет"), " ")
	for _, want := range []string{"--vits-model=" + m, "--vits-tokens=" + tok, "--vits-length-scale=0.80", "o.wav", "Привет"} {
		if !strings.Contains(a, want) {
			t.Errorf("args %q lack %q", a, want)
		}
	}
}
