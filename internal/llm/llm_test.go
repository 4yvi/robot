package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sse(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, c := range chunks {
		fmt.Fprintf(w, "data: %s\n\n", c)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func TestChatStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream || req.Model != "m" {
			t.Errorf("bad request %+v", req)
		}
		sse(w, `{"choices":[{"delta":{"content":"При"}}]}`, `{"choices":[{"delta":{"content":"вет"}}]}`)
	}))
	defer srv.Close()
	var toks []string
	msg, err := New(srv.URL, "m").Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, func(s string) { toks = append(toks, s) })
	if err != nil || msg.Content != "Привет" || len(toks) != 2 {
		t.Fatalf("msg=%+v toks=%v err=%v", msg, toks, err)
	}
}

func TestChatToolCallDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"web_search","arguments":"{\"q\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"go\"}"}}]}}]}`,
		)
	}))
	defer srv.Close()
	msg, err := New(srv.URL, "m").Chat(context.Background(), nil, nil, nil)
	if err != nil || len(msg.ToolCalls) != 1 {
		t.Fatalf("msg=%+v err=%v", msg, err)
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "c1" || tc.Function.Name != "web_search" || tc.Function.Arguments != `{"q":"go"}` {
		t.Fatalf("tool call %+v", tc)
	}
}

func TestJSONStructuredOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat == nil || req.Stream {
			t.Errorf("expected non-stream request with response_format")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + "```json\\n{\\\"ok\\\":true}\\n```" + `"}}]}`))
	}))
	defer srv.Close()
	var out struct{ OK bool }
	if err := New(srv.URL, "m").JSON(context.Background(), nil, "x", map[string]any{"type": "object"}, &out); err != nil || !out.OK {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
	}))
	defer srv.Close()
	ids, err := New(srv.URL+"/v1/", "").Models(context.Background())
	if err == nil && len(ids) == 2 {
		return
	}
	t.Fatalf("ids=%v err=%v", ids, err)
}

func TestLoadPromptFallback(t *testing.T) {
	if LoadPrompt("/nonexistent/prompt.md") == "" {
		t.Fatal("expected fallback")
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.81:1234":            "http://192.168.1.81:1234/v1",
		"http://192.168.1.81:1234":     "http://192.168.1.81:1234/v1",
		"http://192.168.1.81:1234/v1/": "http://192.168.1.81:1234/v1",
		"  http://localhost:1234/v1  ": "http://localhost:1234/v1",
	} {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChatPicksFirstModelWhenNoneChosen(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`{"data":[{"id":"first"},{"id":"second"}]}`))
			return
		}
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		got = req.Model
		sse(w, `{"choices":[{"delta":{"content":"ok"}}]}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "").Chat(context.Background(), nil, nil, nil); err != nil || got != "first" {
		t.Fatalf("model=%q err=%v", got, err)
	}
}
