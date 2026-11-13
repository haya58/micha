package tools

import (
	"context"
	"strings"
	"testing"
)

func TestBraveSearchNoKey(t *testing.T) {
	if _, err := BraveSearch(context.Background(), "", "hello", 5); err == nil {
		t.Fatal("expected error without API key")
	}
}

func TestFetchPageBadScheme(t *testing.T) {
	if _, err := FetchPage(context.Background(), "ftp://example.com/x"); err == nil {
		t.Fatal("expected error for non-http scheme")
	}
}

func TestFetchPagePrivateIPBlocked(t *testing.T) {
	// Literal loopback must be rejected without any network access.
	if _, err := FetchPage(context.Background(), "http://127.0.0.1/"); err == nil {
		t.Fatal("expected SSRF rejection for 127.0.0.1")
	} else if !strings.Contains(err.Error(), "private IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStripHTML(t *testing.T) {
	out := stripHTML("<html><body><p>hello <b>world</b></p><script>evil()</script></body></html>")
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Fatalf("text lost: %q", out)
	}
	if strings.Contains(out, "evil") {
		t.Fatalf("script not stripped: %q", out)
	}
}

func TestURLQueryEscapes(t *testing.T) {
	if got := urlQuery("a b&c"); got != "a%20b%26c" {
		t.Fatalf("got %q", got)
	}
}

func TestExecShellRequiresDocker(t *testing.T) {
	// Hide docker binary via PATH so the test never spawns containers.
	t.Setenv("PATH", t.TempDir())
	if _, err := ExecShell(context.Background(), "alpine:latest", "echo hi", 5); err == nil {
		t.Fatal("expected error when docker is unavailable")
	}
}
