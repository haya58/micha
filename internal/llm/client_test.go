package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func toolNames(tools []ToolDef) map[string]bool {
	m := map[string]bool{}
	for _, td := range tools {
		m[td.Function.Name] = true
	}
	return m
}

func TestDefaultToolsWithShell(t *testing.T) {
	names := toolNames(DefaultTools(true))
	for _, want := range []string{"brave_search", "fetch_page", "exec_shell", "remember"} {
		if !names[want] {
			t.Fatalf("missing tool %q", want)
		}
	}
}

func TestDefaultToolsWithoutShell(t *testing.T) {
	tools := DefaultTools(false)
	names := toolNames(tools)
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
	if names["exec_shell"] {
		t.Fatal("exec_shell must be absent when shell is disabled")
	}
	if !names["remember"] {
		t.Fatal("remember must always be present")
	}
}

func testClient(baseURL string, extra map[string]string) *Client {
	c, err := New(
		map[string]Endpoint{"test": {BaseURL: baseURL, APIKey: "key", ExtraHeaders: extra}},
		"test/model", "test/embed",
	)
	if err != nil {
		panic(err)
	}
	return c
}

func TestExtraHeadersSent(t *testing.T) {
	var gotSession, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession, gotAuth = r.Header.Get("x-opencode-session"), r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL, map[string]string{"x-opencode-session": "ses_123"})
	msg, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "hi" {
		t.Fatalf("got %q", msg.Content)
	}
	if gotSession != "ses_123" {
		t.Fatalf("x-opencode-session not sent, got %q", gotSession)
	}
	if gotAuth != "Bearer key" {
		t.Fatalf("Authorization not sent, got %q", gotAuth)
	}
}

func TestVisionMarshal(t *testing.T) {
	m := ChatMessage{Role: "user", Content: "see this", Images: []string{"https://cdn.example/i.png"}}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"type":"image_url"`) || !strings.Contains(s, "https://cdn.example/i.png") {
		t.Fatalf("image part missing: %s", s)
	}
	if !strings.Contains(s, `"text":"see this"`) && !strings.Contains(s, `"text": "see this"`) {
		t.Fatalf("text part missing: %s", s)
	}
}

func TestVisionUnmarshalRoundTrip(t *testing.T) {
	in := `{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"https://cdn.example/i.png"}}]}`
	var m ChatMessage
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatal(err)
	}
	if m.Content != "hi" || len(m.Images) != 1 || m.Images[0] != "https://cdn.example/i.png" {
		t.Fatalf("got %+v", m)
	}
}

func TestPlainStringStillWorks(t *testing.T) {
	m := ChatMessage{Role: "user", Content: "hello"}
	raw, _ := json.Marshal(m)
	if !strings.Contains(string(raw), `"content":"hello"`) {
		t.Fatalf("plain content broken: %s", raw)
	}
	var back ChatMessage
	if err := json.Unmarshal(raw, &back); err != nil || back.Content != "hello" {
		t.Fatalf("round trip failed: %+v %v", back, err)
	}
}

func TestChatFallsBackToCompletionTokens(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		if strings.Contains(string(raw), `"max_tokens"`) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.","type":"invalid_request_error"}}`))
			return
		}
		if strings.Contains(string(raw), `"temperature"`) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 0.8 with this model.","type":"invalid_request_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL, nil)
	msg, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "ok" {
		t.Fatalf("got %q", msg.Content)
	}
	if len(bodies) != 3 {
		t.Fatalf("expected 2 retries, got %d requests", len(bodies))
	}
	if !strings.Contains(bodies[0], `"max_tokens"`) || !strings.Contains(bodies[0], `"temperature"`) {
		t.Fatalf("first request should use legacy params: %s", bodies[0])
	}
	last := bodies[len(bodies)-1]
	if !strings.Contains(last, `"max_completion_tokens"`) || strings.Contains(last, `"temperature"`) {
		t.Fatalf("final retry should swap params: %s", last)
	}
}

func TestChatReasoningEffortFallback(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		if !strings.Contains(string(raw), `"reasoning_effort":"none"`) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Function tools with reasoning_effort are not supported. Use /v1/responses or set reasoning_effort to 'none'.","type":"invalid_request_error","param":"reasoning_effort"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL, nil)
	msg, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, DefaultTools(true))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "ok" {
		t.Fatalf("got %q", msg.Content)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 1 retry, got %d requests", len(bodies))
	}
	if strings.Contains(bodies[0], "reasoning_effort") {
		t.Fatalf("first request must not set reasoning_effort: %s", bodies[0])
	}
}

func TestChatUnrelated400DoesNotRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL, nil)
	if _, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, nil); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("unrelated 400 must not retry, got %d calls", calls)
	}
}
