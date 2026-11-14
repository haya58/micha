// Package llm is a minimal OpenAI-compatible client (chat + embeddings)
// so any OpenAI-compatible endpoint can be used.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dis-llm-bot/internal/textutil"
)

type Client struct {
	BaseURL      string
	APIKey       string
	Model        string
	EmbedModel   string
	ExtraHeaders map[string]string
	HTTP         *http.Client
}

func New(baseURL, apiKey, model, embedModel string, extraHeaders map[string]string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL, APIKey: apiKey, Model: model, EmbedModel: embedModel,
		ExtraHeaders: extraHeaders,
		HTTP:         &http.Client{Timeout: 120 * time.Second},
	}
}

// setHeaders applies default auth headers plus endpoint-specific extras
// (e.g. x-opencode-session required by some OpenAI-compatible gateways).
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	for k, v := range c.ExtraHeaders {
		req.Header.Set(k, v)
	}
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

func DefaultTools(shellEnabled bool) []ToolDef {
	tools := []ToolDef{
		{
			Type: "function",
		},
		{
			Type: "function",
		},
	}
	tools[0].Function.Name = "brave_search"
	tools[0].Function.Description = "Search the web with Brave. Use for current events, facts, docs."
	tools[0].Function.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
			"count": map[string]any{"type": "integer"},
		},
		"required": []string{"query"},
	}
	tools[1].Function.Name = "fetch_page"
	tools[1].Function.Description = "Fetch a user-posted URL and extract readable text. Rejects private IPs."
	tools[1].Function.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string"},
		},
		"required": []string{"url"},
	}
	if shellEnabled {
		t := ToolDef{Type: "function"}
		t.Function.Name = "exec_shell"
		t.Function.Description = "Run a short non-interactive shell command in a disposable container with NO network. Timeout in seconds."
		t.Function.Parameters = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
				"timeout": map[string]any{"type": "integer"},
			},
			"required": []string{"command"},
		}
		tools = append(tools, t)
	}
	return tools
}

type chatReq struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []ToolDef     `json:"tools,omitempty"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResp struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Chat(ctx context.Context, msgs []ChatMessage, tools []ToolDef) (ChatMessage, error) {
	body, _ := json.Marshal(chatReq{
		Model: c.Model, Messages: msgs, Tools: tools,
		Temperature: 0.8, MaxTokens: 1500,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	c.setHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return ChatMessage{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return ChatMessage{}, fmt.Errorf("chat status %d: %s", resp.StatusCode, textutil.Truncate(string(raw), 300))
	}
	var out chatResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return ChatMessage{}, fmt.Errorf("chat decode: %v body=%s", err, textutil.Truncate(string(raw), 300))
	}
	if out.Error != nil {
		return ChatMessage{}, fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return ChatMessage{}, fmt.Errorf("llm: no choices body=%s", textutil.Truncate(string(raw), 300))
	}
	return out.Choices[0].Message, nil
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": c.EmbedModel, "input": texts})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/embeddings", bytes.NewReader(body))
	c.setHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed status %d: %s (endpoint may not offer /embeddings)", resp.StatusCode, textutil.Truncate(string(raw), 300))
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("embed decode: %v", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("embed error: %s", out.Error.Message)
	}
	res := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		res[i] = d.Embedding
	}
	return res, nil
}
