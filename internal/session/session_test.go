package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/4yvi/robot/internal/clarify"
	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/llm"
	"github.com/4yvi/robot/internal/memory"
	"github.com/4yvi/robot/internal/tools"
	"github.com/4yvi/robot/internal/tts"
	"github.com/4yvi/robot/internal/wake"
)

type fakeDet struct{}

func (fakeDet) Accept([]int16) bool { return false }
func (fakeDet) Reset()              {}
func (fakeDet) Close()              {}

type fakeSynth struct{}

func (fakeSynth) Synthesize(_ context.Context, t string) ([]byte, error) { return []byte(t), nil }

type fakePlayer struct {
	mu    sync.Mutex
	heard []string
}

func (p *fakePlayer) Play(_ context.Context, wav []byte) error {
	p.mu.Lock()
	p.heard = append(p.heard, string(wav))
	p.mu.Unlock()
	return nil
}

func (p *fakePlayer) said() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.heard, " | ")
}

// scriptedLLM returns queued replies for Chat and a function for JSON.
type scriptedLLM struct {
	mu        sync.Mutex
	replies   []llm.Message
	toolsSeen []int
	jsonFn    func(name string, out any) error
}

func (s *scriptedLLM) Chat(_ context.Context, _ []llm.Message, defs []llm.ToolDef, onToken func(string)) (llm.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolsSeen = append(s.toolsSeen, len(defs))
	var r llm.Message
	if len(s.replies) > 0 {
		r, s.replies = s.replies[0], s.replies[1:]
	} else {
		r = llm.Message{Role: "assistant", Content: "Готово."}
	}
	if r.Content != "" {
		onToken(r.Content)
	}
	return r, nil
}

func (s *scriptedLLM) JSON(_ context.Context, _ []llm.Message, name string, _ any, out any) error {
	if s.jsonFn == nil {
		return json.Unmarshal([]byte(`{"needs_clarification":false,"missing":"","question":""}`), out)
	}
	return s.jsonFn(name, out)
}
func (s *scriptedLLM) Models(context.Context) ([]string, error) { return nil, nil }

type rig struct {
	m      *Manager
	llm    *scriptedLLM
	player *fakePlayer
	store  *memory.SQLite
	cfg    *config.Config
}

func newRig(t *testing.T) *rig {
	t.Helper()
	cfg := config.Default()
	cfg.NotesFile = filepath.Join(t.TempDir(), "notes.md")
	store, err := memory.Open(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	l := &scriptedLLM{}
	p := &fakePlayer{}
	env := &tools.Env{Config: cfg}
	m, err := New(Deps{
		Cfg:         cfg,
		Player:      p,
		NewDetector: func(string) (wake.Detector, error) { return fakeDet{}, nil },
		LLM:         l,
		Clarify:     &clarify.Step{Checker: clarify.LLM{Client: l}, Max: cfg.MaxClarifications},
		Tools:       tools.NewDefault(env),
		Speaker:     &tts.Speaker{Synth: fakeSynth{}, Player: p},
		Store:       store,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{m: m, llm: l, player: p, store: store, cfg: cfg}
}

func (r *rig) newSession(t *testing.T) *sess {
	t.Helper()
	s, err := r.m.startOrResume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPlainAnswerIsSpokenAndStored(t *testing.T) {
	r := newRig(t)
	r.llm.replies = []llm.Message{{Role: "assistant", Content: "Сейчас солнечно и тепло."}}
	s := r.newSession(t)
	if err := r.m.respond(context.Background(), s, "какая погода"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.player.said(), "Сейчас солнечно и тепло.") {
		t.Fatalf("said %q", r.player.said())
	}
	msgs, _ := r.store.Messages(context.Background(), s.id)
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("stored %+v", msgs)
	}
}

func TestClarificationAskedThenSkipped(t *testing.T) {
	r := newRig(t)
	checks := 0
	r.llm.jsonFn = func(name string, out any) error {
		checks++
		return json.Unmarshal([]byte(`{"needs_clarification":true,"missing":"модель","question":"Какая у вас машина?"}`), out)
	}
	s := r.newSession(t)
	if err := r.m.respond(context.Background(), s, "какие диски подойдут на машину"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.player.said(), "Какая у вас машина?") || !s.clarifyPending {
		t.Fatalf("said %q pending=%v", r.player.said(), s.clarifyPending)
	}
	r.llm.replies = []llm.Message{{Role: "assistant", Content: "Для Камри подойдут семнадцатые."}}
	if err := r.m.respond(context.Background(), s, "тойота камри"); err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("clarification check must be skipped on the answer, ran %d times", checks)
	}
	if !strings.Contains(r.player.said(), "Камри") || s.clarifyPending {
		t.Fatalf("said %q", r.player.said())
	}
}

func TestToolLoop(t *testing.T) {
	r := newRig(t)
	r.llm.replies = []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Type: "function", Function: llm.FunctionCall{Name: "current_time", Arguments: "{}"}}}},
		{Role: "assistant", Content: "Сейчас вечер."},
	}
	s := r.newSession(t)
	if err := r.m.respond(context.Background(), s, "сколько времени"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.player.said(), "Сейчас вечер.") {
		t.Fatalf("said %q", r.player.said())
	}
	msgs, _ := r.store.Messages(context.Background(), s.id)
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "user,tool_call,tool,assistant" {
		t.Fatalf("roles %v", roles)
	}
}

func TestToolCallLimit(t *testing.T) {
	r := newRig(t)
	r.cfg.MaxToolCalls = 2
	call := llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "x", Type: "function", Function: llm.FunctionCall{Name: "current_time", Arguments: "{}"}}}}
	r.llm.replies = []llm.Message{call, call, call, call, call}
	s := r.newSession(t)
	done := make(chan error, 1)
	go func() { done <- r.m.respond(context.Background(), s, "время") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tool loop did not stop")
	}
	// Two calls allowed: tools offered twice, then withheld.
	if len(r.llm.toolsSeen) != 3 || r.llm.toolsSeen[2] != 0 {
		t.Fatalf("tools offered per request: %v", r.llm.toolsSeen)
	}
}

func TestSummaryAndMemoryCarryOver(t *testing.T) {
	r := newRig(t)
	r.llm.jsonFn = func(name string, out any) error {
		if name == "session_summary" {
			return json.Unmarshal([]byte(`{"summary":"Обсуждали выбор дисков для Тойота Камри.","facts":["У пользователя Тойота Камри"]}`), out)
		}
		return json.Unmarshal([]byte(`{"needs_clarification":false,"missing":"","question":""}`), out)
	}
	s := r.newSession(t)
	_ = r.m.respond(context.Background(), s, "какие диски подойдут на камри")
	r.m.finalize(s)
	r.m.sessMu.Lock()
	r.m.cur = nil // the session ended (stop phrase or timeout)
	r.m.sessMu.Unlock()

	s2 := r.newSession(t)
	if s2.id == s.id {
		t.Fatal("expected a new session")
	}
	sys := r.m.buildMessages(s2)[0].Content
	if !strings.Contains(sys, "Обсуждали выбор дисков") || !strings.Contains(sys, "У пользователя Тойота Камри") {
		t.Fatalf("memory not injected into the system prompt:\n%s", sys)
	}
	if !strings.Contains(sys, "Сейчас:") {
		t.Fatal("current time missing")
	}
}

func TestSessionResumeAndExpiry(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	s1 := r.newSession(t)
	r.m.armExpiry(s1)
	s2, _ := r.m.startOrResume(ctx)
	if s2 != s1 {
		t.Fatal("recent session must be resumed")
	}
	s2.last = time.Now().Add(-11 * time.Minute)
	s3, _ := r.m.startOrResume(ctx)
	if s3 == s1 {
		t.Fatal("stale session must not be resumed")
	}
}

func TestStopAndYes(t *testing.T) {
	stops := []string{"хватит", "пока", "стоп"}
	for _, ok := range []string{"хватит", "Пока!", "ну стоп", "стоп."} {
		if !IsStop(ok, stops) {
			t.Errorf("%q should stop", ok)
		}
	}
	for _, no := range []string{"расскажи про пока и хватит", "какая погода", ""} {
		if IsStop(no, stops) {
			t.Errorf("%q should not stop", no)
		}
	}
	if !IsYes("Да, выполняй") || IsYes("нет") || IsYes("да нет") || IsYes("может быть") {
		t.Error("IsYes misclassified")
	}
}
