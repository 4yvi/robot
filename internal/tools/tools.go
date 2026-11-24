// Package tools implements the functions the model can call.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/llm"
)

// Tool is one callable function.
type Tool interface {
	Name() string
	Def() llm.ToolDef
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// MemorySearcher searches past conversations.
type MemorySearcher interface {
	Search(ctx context.Context, query string, limit int) ([]string, error)
}

// Env carries what tools need from the rest of the app.
type Env struct {
	Config *config.Config
	// Say speaks text asynchronously (used by timers and reminders).
	Say func(text string)
	// Confirm asks the user a yes/no question by voice and returns the answer.
	Confirm func(ctx context.Context, question string) (bool, error)
	Memory  MemorySearcher
	HTTP    *http.Client
	Now     func() time.Time
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Env) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// Registry holds the available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds a tool.
func (r *Registry) Register(t Tool) { r.tools[t.Name()] = t }

// Defs returns tool definitions for the model, sorted by name.
func (r *Registry) Defs() []llm.ToolDef {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	defs := make([]llm.ToolDef, 0, len(names))
	for _, n := range names {
		defs = append(defs, r.tools[n].Def())
	}
	return defs
}

// Names lists registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Call runs a tool call and returns text for the model. Errors become text so
// the model can react to them.
func (r *Registry) Call(ctx context.Context, tc llm.ToolCall) string {
	t, ok := r.tools[tc.Function.Name]
	if !ok {
		return "error: unknown tool " + tc.Function.Name
	}
	out, err := t.Run(ctx, json.RawMessage(tc.Function.Arguments))
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

// funcTool adapts a plain function to Tool.
type funcTool struct {
	name, desc string
	props      map[string]any
	required   []string
	run        func(ctx context.Context, args json.RawMessage) (string, error)
}

func (f *funcTool) Name() string { return f.name }
func (f *funcTool) Def() llm.ToolDef {
	params := map[string]any{"type": "object", "properties": f.props}
	if len(f.required) > 0 {
		params["required"] = f.required
	}
	return llm.ToolDef{Type: "function", Function: llm.FuncDef{Name: f.name, Description: f.desc, Parameters: params}}
}
func (f *funcTool) Run(ctx context.Context, a json.RawMessage) (string, error) { return f.run(ctx, a) }

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }

func decode(args json.RawMessage, v any) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, v); err != nil {
		return fmt.Errorf("bad arguments: %w", err)
	}
	return nil
}

// truncate cuts s to at most n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// NewDefault registers every built-in tool.
func NewDefault(env *Env) *Registry {
	r := NewRegistry()
	sched := newScheduler(env)
	for _, t := range []Tool{
		webSearchTool(env), fetchPageTool(env), currentTimeTool(env),
		sched.timerTool(), sched.reminderTool(),
		openTool(env), mediaTool(env), clipboardReadTool(), clipboardWriteTool(),
		filesSearchTool(env), fileReadTool(env), noteAddTool(env),
		shellTool(env), memorySearchTool(env),
	} {
		r.Register(t)
	}
	return r
}
