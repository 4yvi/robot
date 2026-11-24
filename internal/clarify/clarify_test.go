package clarify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/4yvi/robot/internal/llm"
)

type fakeLLM struct {
	reply string
	err   error
	last  []llm.Message
}

func (f *fakeLLM) Chat(context.Context, []llm.Message, []llm.ToolDef, func(string)) (llm.Message, error) {
	return llm.Message{}, nil
}
func (f *fakeLLM) JSON(_ context.Context, m []llm.Message, _ string, _ any, out any) error {
	f.last = m
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.reply), out)
}
func (f *fakeLLM) Models(context.Context) ([]string, error) { return nil, nil }

func TestCheckAsksForCarModel(t *testing.T) {
	f := &fakeLLM{reply: `{"needs_clarification":true,"missing":"модель машины","question":"Какая у вас модель машины?"}`}
	r, err := LLM{f}.Check(context.Background(), nil, "какие диски подойдут на машину")
	if err != nil || !r.NeedsClarification || r.Question != "Какая у вас модель машины?" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if got := f.last[len(f.last)-1].Content; got != "Вопрос пользователя: какие диски подойдут на машину" {
		t.Fatalf("prompt %q", got)
	}
}

func TestEmptyQuestionMeansNoClarification(t *testing.T) {
	f := &fakeLLM{reply: `{"needs_clarification":true,"missing":"x","question":"  "}`}
	r, _ := LLM{f}.Check(context.Background(), nil, "q")
	if r.NeedsClarification {
		t.Fatal("should not clarify without a question")
	}
}

type stub struct{ calls int }

func (s *stub) Check(context.Context, []llm.Message, string) (Result, error) {
	s.calls++
	return Result{NeedsClarification: true, Question: "а какая?"}, nil
}

func TestStepSkipsWhenAnswering(t *testing.T) {
	c := &stub{}
	st := &Step{Checker: c, Max: 2}
	q, _ := st.Evaluate(context.Background(), nil, "диски", false)
	if q == "" || c.calls != 1 {
		t.Fatalf("q=%q calls=%d", q, c.calls)
	}
	q, _ = st.Evaluate(context.Background(), nil, "Тойота Камри", true)
	if q != "" || c.calls != 1 {
		t.Fatalf("check must be skipped: q=%q calls=%d", q, c.calls)
	}
}

func TestStepFallbackOnError(t *testing.T) {
	st := &Step{Checker: LLM{&fakeLLM{err: errors.New("down")}}, Max: 2}
	if _, err := st.Evaluate(context.Background(), nil, "q", false); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestStepMax(t *testing.T) {
	c := &stub{}
	st := &Step{Checker: c, Max: 1}
	_, _ = st.Evaluate(context.Background(), nil, "a", false)
	q, _ := st.Evaluate(context.Background(), nil, "b", false)
	if q != "" {
		t.Fatal("cap not enforced")
	}
}
