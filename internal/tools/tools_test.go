package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/llm"
)

func testEnv(t *testing.T) *Env {
	t.Helper()
	cfg := config.Default()
	cfg.NotesFile = filepath.Join(t.TempDir(), "notes.md")
	return &Env{Config: cfg, Now: func() time.Time { return time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC) }}
}

func call(r *Registry, name, args string) string {
	return r.Call(context.Background(), llm.ToolCall{Function: llm.FunctionCall{Name: name, Arguments: args}})
}

func jsonArgs(k, v string) string {
	b, _ := json.Marshal(map[string]string{k: v})
	return string(b)
}

func TestParseDuckDuckGo(t *testing.T) {
	page := `<div class="result"><h2><a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa&rut=x">Заголовок <b>один</b></a></h2>
<a class="result__snippet" href="x">Описание  первого</a></div>
<div class="result"><a class="result__a" href="https://b.org/">Два</a><a class="result__snippet">Описание два</a></div>`
	res, err := ParseDuckDuckGo(strings.NewReader(page))
	if err != nil || len(res) != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res[0].URL != "https://example.com/a" || res[0].Title != "Заголовок один" || res[0].Snippet != "Описание первого" {
		t.Fatalf("bad first result %+v", res[0])
	}
}

func TestExtractText(t *testing.T) {
	got, _ := ExtractText(strings.NewReader(`<html><head><style>p{}</style></head><body><nav>menu</nav><h1>Тема</h1><p>Первый   абзац</p><script>evil()</script><p>Второй</p></body></html>`))
	if got != "Тема\nПервый абзац\nВторой" {
		t.Fatalf("got %q", got)
	}
}

func TestAllowedPath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0o600)
	_ = os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600)
	_ = os.Symlink(outside, filepath.Join(root, "link"))

	if _, err := AllowedPath([]string{root}, filepath.Join(root, "a.txt")); err != nil {
		t.Fatalf("inside path rejected: %v", err)
	}
	if _, err := AllowedPath([]string{root}, filepath.Join(root, "..", filepath.Base(outside), "secret.txt")); err == nil {
		t.Fatal("traversal allowed")
	}
	if _, err := AllowedPath([]string{root}, filepath.Join(root, "link", "secret.txt")); err == nil {
		t.Fatal("symlink escape allowed")
	}
	if _, err := AllowedPath(nil, root); err == nil {
		t.Fatal("empty allow list must deny")
	}
}

func TestFilesTools(t *testing.T) {
	env := testEnv(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "report-2026.txt"), []byte("содержимое"), 0o600)
	env.Config.AllowedFolders = []string{root}
	r := NewDefault(env)
	if out := call(r, "files_search", `{"query":"REPORT"}`); !strings.Contains(out, "report-2026.txt") {
		t.Fatalf("search: %s", out)
	}
	if out := call(r, "file_read", jsonArgs("path", filepath.Join(root, "report-2026.txt"))); out != "содержимое" {
		t.Fatalf("read: %s", out)
	}
	if out := call(r, "file_read", `{"path":"/etc/hosts"}`); !strings.HasPrefix(out, "error:") {
		t.Fatalf("read outside allowed: %s", out)
	}
}

func TestParseWhen(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	got, err := ParseWhen(now, "15:30")
	if err != nil || !got.Equal(time.Date(2026, 1, 15, 15, 30, 0, 0, time.UTC)) {
		t.Fatalf("got %v %v", got, err)
	}
	got, _ = ParseWhen(now, "09:00")
	if got.Day() != 16 {
		t.Fatalf("past time should roll to tomorrow: %v", got)
	}
	if _, err := ParseWhen(now, "когда-нибудь"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTimerSpeaks(t *testing.T) {
	env := testEnv(t)
	said := make(chan string, 1)
	env.Say = func(s string) { said <- s }
	r := NewDefault(env)
	if out := call(r, "set_timer", `{"seconds":0.05,"label":"чай"}`); strings.HasPrefix(out, "error") {
		t.Fatal(out)
	}
	select {
	case s := <-said:
		if !strings.Contains(s, "чай") {
			t.Fatalf("said %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire")
	}
}

func TestNoteAndTime(t *testing.T) {
	env := testEnv(t)
	r := NewDefault(env)
	call(r, "note_add", `{"text":"купить хлеб"}`)
	b, _ := os.ReadFile(env.Config.NotesFile)
	if !strings.Contains(string(b), "2026-01-15 10:00 купить хлеб") {
		t.Fatalf("note file: %q", b)
	}
	if out := call(r, "current_time", `{}`); !strings.Contains(out, "четверг") || !strings.Contains(out, "15.01.2026 10:00") {
		t.Fatalf("time: %s", out)
	}
}

func TestShellNeedsConfirmation(t *testing.T) {
	env := testEnv(t)
	r := NewDefault(env)
	if out := call(r, "run_shell", `{"command":"echo hi"}`); !strings.HasPrefix(out, "error") {
		t.Fatalf("must refuse without confirmer: %s", out)
	}
	asked := ""
	env.Confirm = func(_ context.Context, q string) (bool, error) { asked = q; return false, nil }
	if out := call(r, "run_shell", `{"command":"touch /tmp/should-not-exist-robot"}`); !strings.Contains(out, "не подтвердил") || asked == "" {
		t.Fatalf("out=%s asked=%q", out, asked)
	}
	if _, err := os.Stat("/tmp/should-not-exist-robot"); err == nil {
		t.Fatal("command ran despite refusal")
	}
	env.Confirm = func(context.Context, string) (bool, error) { return true, nil }
	if out := call(r, "run_shell", `{"command":"echo hello"}`); !strings.Contains(out, "hello") {
		t.Fatalf("out=%s", out)
	}
}

func TestRegistry(t *testing.T) {
	r := NewDefault(testEnv(t))
	want := []string{"clipboard_read", "clipboard_write", "current_time", "fetch_page", "file_read", "files_search",
		"media_control", "memory_search", "note_add", "open_app_or_url", "run_shell", "set_reminder", "set_timer", "web_search"}
	got := r.Names()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names %v", got)
	}
	for _, d := range r.Defs() {
		if _, err := json.Marshal(d); err != nil {
			t.Fatal(err)
		}
	}
	if out := call(r, "nope", `{}`); !strings.HasPrefix(out, "error") {
		t.Fatal(out)
	}
	if out := call(r, "media_control", `{"action":"explode"}`); !strings.HasPrefix(out, "error") {
		t.Fatal(out)
	}
}

func TestOpenRejectsUnknown(t *testing.T) {
	r := NewDefault(testEnv(t))
	if out := call(r, "open_app_or_url", `{"target":"file:///etc/passwd"}`); !strings.HasPrefix(out, "error") {
		t.Fatalf("file URL must be rejected: %s", out)
	}
}
