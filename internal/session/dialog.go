package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/4yvi/robot/internal/llm"
	"github.com/4yvi/robot/internal/memory"
)

const (
	maxHistory     = 40 // messages sent to the model
	firstWaitMS    = 6000
	recentSessions = 3
)

var weekdays = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

// sess is one dialog session (it may span several wake-word activations).
type sess struct {
	id             int64
	history        []llm.Message
	clarifyPending bool
	last           time.Time
	memoryBlock    string
	timer          *time.Timer
}

// startOrResume continues the current session if the last turn was recent
// enough, otherwise closes it and begins a new one.
func (m *Manager) startOrResume(ctx context.Context) (*sess, error) {
	timeout := time.Duration(m.d.Cfg.SessionTimeoutMinutes) * time.Minute
	m.sessMu.Lock()
	cur := m.cur
	if cur != nil {
		if cur.timer != nil {
			cur.timer.Stop()
		}
		if time.Since(cur.last) < timeout {
			m.busy = true
			m.sessMu.Unlock()
			slog.Info("resuming session", "id", cur.id)
			return cur, nil
		}
		m.cur = nil
		go m.finalize(cur)
	}
	m.sessMu.Unlock()

	id, err := m.d.Store.StartSession(ctx)
	if err != nil {
		return nil, err
	}
	s := &sess{id: id, last: time.Now(), memoryBlock: m.memoryBlock(ctx)}
	m.sessMu.Lock()
	m.cur = s
	m.busy = true
	m.sessMu.Unlock()
	slog.Info("new session", "id", id)
	return s, nil
}

// memoryBlock builds the system-prompt addition from past summaries and facts.
func (m *Manager) memoryBlock(ctx context.Context) string {
	var b strings.Builder
	if sums, err := m.d.Store.RecentSummaries(ctx, recentSessions); err == nil && len(sums) > 0 {
		b.WriteString("\n\nКратко о прошлых беседах с пользователем (от старых к новым):\n")
		for _, s := range sums {
			b.WriteString("- " + s + "\n")
		}
	}
	if facts, err := m.d.Store.Facts(ctx); err == nil && len(facts) > 0 {
		b.WriteString("\nВажные факты о пользователе:\n")
		for _, f := range facts {
			b.WriteString("- " + f + "\n")
		}
	}
	return b.String()
}

func (m *Manager) endSession(s *sess) {
	m.sessMu.Lock()
	if m.cur == s {
		m.cur = nil
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	m.sessMu.Unlock()
	go m.finalize(s)
}

// armExpiry closes the session after the configured idle time.
func (m *Manager) armExpiry(s *sess) {
	d := time.Duration(m.d.Cfg.SessionTimeoutMinutes) * time.Minute
	m.sessMu.Lock()
	defer m.sessMu.Unlock()
	m.busy = false
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(d, func() {
		m.sessMu.Lock()
		stale := m.cur == s && !m.busy
		if stale {
			m.cur = nil
		}
		m.sessMu.Unlock()
		if stale {
			m.finalize(s)
		}
	})
}

type summaryResult struct {
	Summary string   `json:"summary"`
	Facts   []string `json:"facts"`
}

var summarySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{"type": "string"},
		"facts":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"summary", "facts"},
	"additionalProperties": false,
}

const summaryInstruction = `Ниже диалог пользователя с голосовым ассистентом.
Сделай краткую сводку на русском в одном-трёх предложениях: о чём говорили и к чему пришли.
Затем выпиши список важных устойчивых фактов о пользователе (имя, предпочтения, техника, семья, проекты), которые пригодятся в будущих беседах. Не включай временные детали. Если фактов нет, верни пустой список.`

// finalize summarizes a finished session and stores the result.
func (m *Manager) finalize(s *sess) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var transcript strings.Builder
	for _, msg := range s.history {
		switch msg.Role {
		case "user":
			transcript.WriteString("Пользователь: " + msg.Content + "\n")
		case "assistant":
			if msg.Content != "" {
				transcript.WriteString("Ассистент: " + msg.Content + "\n")
			}
		}
	}
	var res summaryResult
	if strings.TrimSpace(transcript.String()) != "" {
		err := m.d.LLM.JSON(ctx, []llm.Message{
			{Role: "system", Content: summaryInstruction},
			{Role: "user", Content: transcript.String()},
		}, "session_summary", summarySchema, &res)
		if err != nil {
			slog.Warn("summarize session", "id", s.id, "err", err)
		}
	}
	if err := m.d.Store.EndSession(ctx, s.id, strings.TrimSpace(res.Summary)); err != nil {
		slog.Error("end session", "err", err)
	}
	if err := m.d.Store.AddFacts(ctx, res.Facts); err != nil {
		slog.Error("save facts", "err", err)
	}
	slog.Info("session closed", "id", s.id, "facts", len(res.Facts))
}

// handleSession runs one activation of the wake word: it listens, answers and
// keeps listening for follow-ups until silence, a stop phrase or an error.
func (m *Manager) handleSession(ctx context.Context) {
	s, err := m.startOrResume(ctx)
	if err != nil {
		slog.Error("start session", "err", err)
		return
	}
	defer m.armExpiry(s)
	m.beep(ctx)

	wait := firstWaitMS
	for ctx.Err() == nil {
		text, err := m.listen(ctx, wait)
		if err != nil {
			slog.Error("listen", "err", err)
			m.speakText(ctx, "Не получилось распознать речь.")
			return
		}
		if text == "" {
			return // silence: the session goes to waiting
		}
		if m.d.OnUserText != nil {
			m.d.OnUserText(text)
		}
		if IsStop(text, m.d.Cfg.StopPhrases) {
			m.speakText(ctx, "Хорошо, до связи.")
			m.endSession(s)
			return
		}

		turnCtx, cancel := context.WithCancel(ctx)
		bg := newBargeGate()
		restore := m.setRoute(func(c []int16) { // the wake word interrupts the answer
			hit := m.detAccept(c)
			if loud := bg.feed(c); hit && loud && m.d.Cfg.BargeIn {
				cancel()
			} else if hit {
				slog.Debug("ignored wake word heard during the answer (likely echo)")
			}
		})
		err = m.respond(turnCtx, s, text)
		restore()
		interrupted := turnCtx.Err() != nil && ctx.Err() == nil
		cancel()
		s.last = time.Now()
		if interrupted {
			slog.Info("answer interrupted by wake word")
			m.detReset()
			m.beep(ctx)
			wait = firstWaitMS
			continue
		}
		if err != nil {
			slog.Error("respond", "err", err)
			m.speakText(ctx, "Не получилось получить ответ от модели.")
			return
		}
		sleep(ctx, 250*time.Millisecond)
		wait = m.d.Cfg.FollowUpSeconds * 1000
	}
}

func (s *sess) add(m *Manager, ctx context.Context, msg llm.Message, rec memory.Message) {
	s.history = append(s.history, msg)
	if err := m.d.Store.AddMessage(context.WithoutCancel(ctx), s.id, rec); err != nil {
		slog.Error("save message", "err", err)
	}
}

// respond handles one user utterance: optional clarification, then the answer.
func (m *Manager) respond(ctx context.Context, s *sess, text string) error {
	answering := s.clarifyPending
	s.clarifyPending = false
	m.setState(StateThinking)

	question, err := m.d.Clarify.Evaluate(ctx, s.history, text, answering)
	if err != nil {
		slog.Warn("clarification check failed, answering directly", "err", err)
		question = ""
	}
	s.add(m, ctx, llm.Message{Role: "user", Content: text}, memory.Message{Role: "user", Content: text})
	if question != "" {
		s.clarifyPending = true
		s.add(m, ctx, llm.Message{Role: "assistant", Content: question}, memory.Message{Role: "assistant", Content: question})
		m.speakText(ctx, question)
		return nil
	}
	return m.answer(ctx, s)
}

// buildMessages composes the system prompt, memory and recent history.
func (m *Manager) buildMessages(s *sess) []llm.Message {
	now := time.Now()
	system := llm.LoadPrompt(m.d.Cfg.ResolvePath(m.d.Cfg.PromptPath)) +
		fmt.Sprintf("\n\nСейчас: %s, %s.", weekdays[now.Weekday()], now.Format("02.01.2006 15:04")) +
		s.memoryBlock
	h := s.history
	if len(h) > maxHistory {
		h = h[len(h)-maxHistory:]
		for len(h) > 0 && h[0].Role != "user" { // do not start with an orphan tool result
			h = h[1:]
		}
	}
	return append([]llm.Message{{Role: "system", Content: system}}, h...)
}

// answer runs the model with tools until it produces a final spoken reply.
func (m *Manager) answer(ctx context.Context, s *sess) error {
	maxCalls := m.d.Cfg.MaxToolCalls
	used := 0
	for {
		var defs []llm.ToolDef
		if used < maxCalls {
			defs = m.d.Tools.Defs()
		}
		msg, err := m.chatAndSpeak(ctx, s, defs)
		if err != nil {
			return err
		}
		if len(msg.ToolCalls) == 0 || defs == nil {
			s.add(m, ctx, llm.Message{Role: "assistant", Content: msg.Content}, memory.Message{Role: "assistant", Content: msg.Content})
			return nil
		}
		msg.Content = strings.TrimSpace(msg.Content)
		s.history = append(s.history, msg)
		if msg.Content != "" {
			_ = m.d.Store.AddMessage(context.WithoutCancel(ctx), s.id, memory.Message{Role: "assistant", Content: msg.Content})
		}
		m.setState(StateThinking)
		for _, tc := range msg.ToolCalls {
			var result string
			if used >= maxCalls {
				result = "error: tool call limit reached, answer with what you have"
			} else {
				slog.Info("tool call", "name", tc.Function.Name, "args", tc.Function.Arguments)
				result = m.d.Tools.Call(ctx, tc)
			}
			used++
			s.history = append(s.history, llm.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result})
			_ = m.d.Store.AddMessage(context.WithoutCancel(ctx), s.id, memory.Message{Role: "tool_call", Content: tc.Function.Arguments, ToolName: tc.Function.Name})
			_ = m.d.Store.AddMessage(context.WithoutCancel(ctx), s.id, memory.Message{Role: "tool", Content: result, ToolName: tc.Function.Name})
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// chatAndSpeak streams the model's reply into the speaker.
func (m *Manager) chatAndSpeak(ctx context.Context, s *sess, defs []llm.ToolDef) (llm.Message, error) {
	tokens := make(chan string, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.speakMu.Lock()
		defer m.speakMu.Unlock()
		if err := m.d.Speaker.SpeakStream(ctx, tokens); err != nil {
			slog.Error("speak", "err", err)
		}
	}()
	first := true
	msg, err := m.d.LLM.Chat(ctx, m.buildMessages(s), defs, func(t string) {
		if first && strings.TrimSpace(t) != "" {
			first = false
			m.setState(StateSpeaking)
		}
		select {
		case tokens <- t:
		case <-ctx.Done():
		}
	})
	close(tokens)
	<-done
	return msg, err
}
