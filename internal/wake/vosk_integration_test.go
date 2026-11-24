//go:build vosk && darwin

package wake

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/4yvi/robot/internal/audio"
)

// Uses the macOS `say` command (voice Milena) to synthesise Russian speech.
func sayPCM(t *testing.T, text string) []int16 {
	t.Helper()
	out := filepath.Join(t.TempDir(), "s.wav")
	if err := exec.Command("say", "-v", "Milena", "-o", out, "--data-format=LEI16@16000", text).Run(); err != nil {
		t.Skip("say/Milena unavailable:", err)
	}
	b, _ := os.ReadFile(out)
	s, _, _, err := audio.DecodeWAV(b)
	if err != nil {
		t.Fatal(err)
	}
	return append(s, make([]int16, 16000)...) // trailing silence
}

func detect(d Detector, pcm []int16) bool {
	hit := false
	for i := 0; i < len(pcm); i += 1600 {
		end := min(i+1600, len(pcm))
		if d.Accept(pcm[i:end]) {
			hit = true
		}
	}
	return hit
}

func TestVoskWakeWord(t *testing.T) {
	model := filepath.Join("..", "..", "models", "vosk-model-small-ru")
	if _, err := os.Stat(model); err != nil {
		t.Skip("vosk model not downloaded")
	}
	d, err := NewVosk(model, "робот", 16000)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !detect(d, sayPCM(t, "робот")) {
		t.Error("wake word was not detected")
	}
	d.Reset()
	if detect(d, sayPCM(t, "сегодня хорошая погода и я иду гулять")) {
		t.Error("unrelated speech triggered the wake word")
	}
}
