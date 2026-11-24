// Package clarify decides whether a question needs a clarifying follow-up.
package clarify

import (
	"context"
	"strings"

	"github.com/4yvi/robot/internal/llm"
)

// Result is the structured verdict from the model.
type Result struct {
	NeedsClarification bool   `json:"needs_clarification"`
	Missing            string `json:"missing"`
	Question           string `json:"question"`
}

// Checker evaluates a user question.
type Checker interface {
	Check(ctx context.Context, history []llm.Message, question string) (Result, error)
}

// Schema is the JSON schema for Result (structured output).
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"needs_clarification": map[string]any{"type": "boolean"},
		"missing":             map[string]any{"type": "string"},
		"question":            map[string]any{"type": "string"},
	},
	"required":             []string{"needs_clarification", "missing", "question"},
	"additionalProperties": false,
}

const instruction = `Ты оцениваешь вопрос пользователя голосовому ассистенту перед ответом.
Реши, можно ли дать полезный ответ без дополнительных данных от пользователя.
Нужно уточнение, только если без него ответ будет бесполезным или случайным, например человек спрашивает, какие диски подойдут на машину, но не назвал модель машины.
Не уточняй то, что можно найти в интернете, угадать по контексту диалога или что не критично.
Верни JSON: needs_clarification (bool), missing (что именно не хватает, коротко), question (один короткий вопрос по-русски для пользователя, который можно произнести вслух). Если уточнение не нужно, missing и question пустые строки.`

// LLM implements Checker with a structured-output request to the model.
type LLM struct{ Client llm.Client }

// Check implements Checker.
func (l LLM) Check(ctx context.Context, history []llm.Message, question string) (Result, error) {
	msgs := []llm.Message{{Role: "system", Content: instruction}}
	if len(history) > 0 {
		var b strings.Builder
		for _, m := range tail(history, 6) {
			if m.Role == "user" || m.Role == "assistant" {
				b.WriteString(m.Role + ": " + m.Content + "\n")
			}
		}
		if b.Len() > 0 {
			msgs = append(msgs, llm.Message{Role: "user", Content: "Контекст диалога:\n" + b.String()})
		}
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: "Вопрос пользователя: " + question})
	var r Result
	if err := l.Client.JSON(ctx, msgs, "clarification", Schema, &r); err != nil {
		return Result{}, err
	}
	r.Question = strings.TrimSpace(r.Question)
	if r.NeedsClarification && r.Question == "" {
		r.NeedsClarification = false // nothing to ask
	}
	return r, nil
}

func tail(m []llm.Message, n int) []llm.Message {
	if len(m) <= n {
		return m
	}
	return m[len(m)-n:]
}

// Step applies the clarification policy around a Checker.
type Step struct {
	Checker Checker
	Max     int // max consecutive clarifications
	count   int
}

// Evaluate returns a question to ask, or "" to go on with the answer.
// answeringClarification is true when the user's text replies to a previous
// clarifying question: the check is skipped then.
func (s *Step) Evaluate(ctx context.Context, history []llm.Message, text string, answeringClarification bool) (string, error) {
	if answeringClarification {
		s.count = 0 // the user answered; the streak ends and the model answers now
		return "", nil
	}
	if s.count >= s.Max {
		s.count = 0
		return "", nil
	}
	r, err := s.Checker.Check(ctx, history, text)
	if err != nil {
		return "", err // caller should fall back to answering directly
	}
	if !r.NeedsClarification {
		s.count = 0
		return "", nil
	}
	s.count++
	return r.Question, nil
}

// Reset starts a fresh clarification streak (call when a new session begins).
func (s *Step) Reset() { s.count = 0 }
