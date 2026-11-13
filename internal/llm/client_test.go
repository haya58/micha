package llm

import "testing"

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
