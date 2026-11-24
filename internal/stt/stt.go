// Package stt is a client for a whisper.cpp whisper-server.
package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Transcriber turns WAV audio into text.
type Transcriber interface {
	Transcribe(ctx context.Context, wav []byte) (string, error)
}

// Client talks to whisper-server over HTTP.
type Client struct {
	BaseURL  string
	Language string
	HTTP     *http.Client
}

// New creates a client for the server at baseURL (e.g. http://127.0.0.1:8080).
func New(baseURL, language string) *Client {
	if language == "" {
		language = "ru"
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Language: language,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
	}
}

// Transcribe posts the WAV to /inference and returns the recognized text.
func (c *Client) Transcribe(ctx context.Context, wav []byte) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(wav); err != nil {
		return "", err
	}
	for k, v := range map[string]string{
		"language":        c.Language,
		"response_format": "json",
		"temperature":     "0.0",
	} {
		if err := w.WriteField(k, v); err != nil {
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/inference", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("whisper-server: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper-server: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("whisper-server: bad response: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("whisper-server: %s", out.Error)
	}
	return Clean(out.Text), nil
}

// Clean trims whitespace and drops whisper's non-speech markers like [BLANK_AUDIO].
func Clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	for {
		i := strings.IndexAny(s, "[(")
		if i < 0 {
			break
		}
		closer := "]"
		if s[i] == '(' {
			closer = ")"
		}
		j := strings.Index(s[i:], closer)
		if j < 0 {
			break
		}
		s = strings.TrimSpace(s[:i] + " " + s[i+j+1:])
	}
	return strings.Join(strings.Fields(s), " ")
}
