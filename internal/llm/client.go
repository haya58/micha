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
	Content    string     `json:"-"`
	Images     []string   `json:"-"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ContentPart is one element of OpenAI multi-part message content.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL references an image for vision-capable models.
type ImageURL struct {
	URL string `json:"url"`
}

// MarshalJSON emits content as a plain string, or as text+image_url parts
// when Images holds attachment URLs.
func (m ChatMessage) MarshalJSON() ([]byte, error) {
	var content any = m.Content
	if len(m.Images) > 0 {
		parts := make([]ContentPart, 0, len(m.Images)+1)
		if m.Content != "" {
			parts = append(parts, ContentPart{Type: "text", Text: m.Content})
		}
		for _, u := range m.Images {
			parts = append(parts, ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: u}})
		}
		content = parts
	}
	return json.Marshal(struct {
		Role       string     `json:"role"`
		Content    any        `json:"content,omitempty"`
		ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
		ToolCallID string     `json:"tool_call_id,omitempty"`
	}{Role: m.Role, Content: content, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID})
}

// UnmarshalJSON accepts both string content and multi-part content.
func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []ToolCall      `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role, m.ToolCalls, m.ToolCallID = raw.Role, raw.ToolCalls, raw.ToolCallID
	if len(raw.Content) == 0 || string(raw.Content) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw.Content, &s); err == nil {
		m.Content = s
		return nil
	}
	var parts []ContentPart
	if err := json.Unmarshal(raw.Content, &parts); err != nil {
		return err
	}
	var texts []string
	for _, p := range parts {
		switch p.Type {
		case "text":
			texts = append(texts, p.Text)
		case "image_url":
			if p.ImageURL != nil {
				m.Images = append(m.Images, p.ImageURL.URL)
			}
		}
	}
	m.Content = strings.Join(texts, "\n")
	return nil
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
	tools[2].Function.Name = "remember"
	tools[2].Function.Description = "Save a durable fact about the user or server for later conversations (preferences, personal details, commitments). Call when the user states something worth recalling, even in a short message."
	tools[2].Function.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"fact":       map[string]any{"type": "string"},
			"importance": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
		},
		"required": []string{"fact"},
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

type chatResp struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Chat(ctx context.Context, msgs []ChatMessage, tools []ToolDef) (ChatMessage, error) {
	// Newer models/endpoints reject max_tokens, non-default temperature,
	// or default reasoning effort combined with function tools.
	// Adjust step by step from the error text, up to 4 attempts.
	key, withTemp := "max_tokens", true
	var reasoningEffort *string
	var err error
	var msg ChatMessage
	for attempt := 0; attempt < 4; attempt++ {
		msg, err = c.postChat(ctx, msgs, tools, key, withTemp, reasoningEffort)
		if err == nil {
			return msg, nil
		}
		lower := strings.ToLower(err.Error())
		changed := false
		if key == "max_tokens" && (strings.Contains(lower, "max_completion_tokens") ||
			(strings.Contains(lower, "max_tokens") && strings.Contains(lower, "unsupported"))) {
			key = "max_completion_tokens"
			changed = true
		}
		if withTemp && strings.Contains(lower, "temperature") {
			withTemp = false
			changed = true
		}
		if reasoningEffort == nil && strings.Contains(lower, "reasoning_effort") {
			none := "none"
			reasoningEffort = &none
			changed = true
		}
		if !changed {
			return ChatMessage{}, err // unrelated error; no retry
		}
	}
	return ChatMessage{}, err
}

func (c *Client) postChat(ctx context.Context, msgs []ChatMessage, tools []ToolDef, maxTokensKey string, withTemp bool, reasoningEffort *string) (ChatMessage, error) {
	payload := map[string]any{
		"model": c.Model, "messages": msgs, maxTokensKey: 1500,
	}
	if withTemp {
		payload["temperature"] = 0.8
	}
	if reasoningEffort != nil {
		payload["reasoning_effort"] = *reasoningEffort
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, _ := json.Marshal(payload)
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
		return nil, fmt.Errorf("embed status %d: %s", resp.StatusCode, textutil.Truncate(string(raw), 300))
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
