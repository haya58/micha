package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	for _, want := range []string{"brave_search", "fetch_page", "exec_shell"} {
		if !names[want] {
			t.Fatalf("missing tool %q", want)
		}
	}
}

func TestDefaultToolsWithoutShell(t *testing.T) {
	tools := DefaultTools(false)
	names := toolNames(tools)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if names["exec_shell"] {
		t.Fatal("exec_shell must be absent when shell is disabled")
	}
}

func TestExtraHeadersSent(t *testing.T) {
	var gotSession, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession, gotAuth = r.Header.Get("x-opencode-session"), r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "key", "model", "embed", map[string]string{"x-opencode-session": "ses_123"})
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
