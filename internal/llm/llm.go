// Package llm is a client for LM Studio's OpenAI-compatible API.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// Message is one chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is a model request to run a tool.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall names the tool and carries JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef describes a callable tool to the model.
type ToolDef struct {
	Type     string  `json:"type"`
	Function FuncDef `json:"function"`
}

// FuncDef is the function part of a ToolDef.
type FuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Client is the interface the rest of the app depends on.
type Client interface {
	// Chat streams a reply, calling onToken for each text piece, and returns
	// the full assistant message (possibly with tool calls).
	Chat(ctx context.Context, msgs []Message, tools []ToolDef, onToken func(string)) (Message, error)
	// JSON asks for a reply that matches the JSON schema and decodes it into out.
	JSON(ctx context.Context, msgs []Message, name string, schema any, out any) error
	Models(ctx context.Context) ([]string, error)
}

// HTTPClient implements Client against LM Studio.
type HTTPClient struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// New creates a client. baseURL looks like http://localhost:1234/v1.
func New(baseURL, model string) *HTTPClient {
	return &HTTPClient{
		BaseURL: NormalizeURL(baseURL),
		Model:   model,
		HTTP:    &http.Client{},
	}
}

// SetModel changes the model used for later requests.
func (c *HTTPClient) SetModel(m string) { c.Model = m }

// SetBaseURL changes the server address.
func (c *HTTPClient) SetBaseURL(u string) { c.BaseURL = NormalizeURL(u) }

// NormalizeURL accepts "192.168.1.5:1234", "http://host:1234" or the full
// ".../v1" form and returns the API base URL ending in /v1.
func NormalizeURL(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" {
		return u
	}
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	if !strings.HasSuffix(u, "/v1") {
		u += "/v1"
	}
	return u
}

// ensureModel picks the first model the server lists when none was chosen,
// because LM Studio rejects requests without a model when nothing is loaded.
func (c *HTTPClient) ensureModel(ctx context.Context) {
	if c.Model != "" {
		return
	}
	if ids, err := c.Models(ctx); err == nil && len(ids) > 0 {
		c.Model = ids[0]
	}
}

type chatRequest struct {
	Model          string    `json:"model,omitempty"`
	Messages       []Message `json:"messages"`
	Tools          []ToolDef `json:"tools,omitempty"`
	Stream         bool      `json:"stream"`
	Temperature    float64   `json:"temperature"`
	ResponseFormat any       `json:"response_format,omitempty"`
}

func (c *HTTPClient) post(ctx context.Context, req chatRequest) (*http.Response, error) {
	if req.Model == "" {
		c.ensureModel(ctx)
		req.Model = c.Model
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return nil, fmt.Errorf("LM Studio: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("LM Studio: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// Chat implements Client with SSE streaming.
func (c *HTTPClient) Chat(ctx context.Context, msgs []Message, tools []ToolDef, onToken func(string)) (Message, error) {
	resp, err := c.post(ctx, chatRequest{Model: c.Model, Messages: msgs, Tools: tools, Stream: true, Temperature: 0.4})
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	out := Message{Role: "assistant"}
	var text strings.Builder
	calls := map[int]*ToolCall{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var ch streamChunk
		if json.Unmarshal([]byte(data), &ch) != nil || len(ch.Choices) == 0 {
			continue
		}
		d := ch.Choices[0].Delta
		if d.Content != "" {
			text.WriteString(d.Content)
			if onToken != nil {
				onToken(d.Content)
			}
		}
		for _, tc := range d.ToolCalls {
			cur := calls[tc.Index]
			if cur == nil {
				cur = &ToolCall{Type: "function"}
				calls[tc.Index] = cur
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			cur.Function.Name += tc.Function.Name
			cur.Function.Arguments += tc.Function.Arguments
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return out, fmt.Errorf("LM Studio stream: %w", err)
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	out.Content = text.String()
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		tc := *calls[i]
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_%d", i)
		}
		if tc.Function.Arguments == "" {
			tc.Function.Arguments = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, tc)
	}
	return out, nil
}

// JSON implements Client using structured output.
func (c *HTTPClient) JSON(ctx context.Context, msgs []Message, name string, schema any, out any) error {
	resp, err := c.post(ctx, chatRequest{
		Model: c.Model, Messages: msgs, Temperature: 0,
		ResponseFormat: map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": name, "strict": true, "schema": schema},
		},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("LM Studio: bad response: %w", err)
	}
	if len(r.Choices) == 0 {
		return fmt.Errorf("LM Studio: empty response")
	}
	content := ExtractJSON(r.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return fmt.Errorf("LM Studio: reply is not valid JSON (%q): %w", content, err)
	}
	return nil
}

// ExtractJSON strips code fences and surrounding text around a JSON object.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

// Models lists model ids from /v1/models.
func (c *HTTPClient) Models(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LM Studio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LM Studio: %s", resp.Status)
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.Data))
	for _, m := range r.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

const fallbackPrompt = "Ты Робот, голосовой ассистент. Отвечай коротко, разговорно, по-русски, без разметки."

// LoadPrompt reads the system prompt file, falling back to a built-in one.
func LoadPrompt(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return fallbackPrompt
	}
	return strings.TrimSpace(string(b))
}
