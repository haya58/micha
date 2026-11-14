package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestStripHTMLStyleAndSpeed(t *testing.T) {
	// style blocks must go too, and large pages must finish fast (linear).
	chunk := "<div><p>text</p><script>var x=1;</script><style>.a{color:red}</style></div>"
	big := "<html><body>" + strings.Repeat(chunk, 2000) + "</body></html>"
	start := time.Now()
	out := stripHTML(big)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("stripHTML too slow: %v", elapsed)
	}
	if strings.Contains(out, "var x") || strings.Contains(out, "color:red") {
		t.Fatalf("script/style leaked: %.100q", out)
	}
	if !strings.Contains(out, "text") {
		t.Fatalf("text lost: %.100q", out)
	}
}

func TestIsBlockedPage(t *testing.T) {
	for _, s := range []string{
		"Vercel Security Checkpoint ...",
		"Just a moment, please verify",
		"VERIFY YOU ARE HUMAN",
		"Access Denied - bot protection",
	} {
		if !isBlockedPage(s) {
			t.Fatalf("should be blocked: %q", s)
		}
	}
	if isBlockedPage("IIJmioの料金プランを紹介します。格安SIMの比較記事です。") {
		t.Fatal("normal article text must not be blocked")
	}
}

func TestBraveQueryEncoding(t *testing.T) {
	var gotQ, gotCount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQ, gotCount = r.URL.Query().Get("q"), r.URL.Query().Get("count")
		if r.Header.Get("X-Subscription-Token") != "testkey" {
			t.Error("missing subscription token header")
		}
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"IIJ","url":"https://example.com","description":"desc"}]}}`))
	}))
	defer srv.Close()

	old := braveSearchBaseURL
	braveSearchBaseURL = srv.URL
	defer func() { braveSearchBaseURL = old }()

	out, err := BraveSearch(context.Background(), "testkey", "インターネットイニシアティブ 企業", 3)
	if err != nil {
		t.Fatalf("BraveSearch: %v", err)
	}
	if gotQ != "インターネットイニシアティブ 企業" {
		t.Fatalf("query not round-tripped, got %q", gotQ)
	}
	if gotCount != "3" {
		t.Fatalf("count not sent, got %q", gotCount)
	}
	if !strings.Contains(out, "IIJ") || !strings.Contains(out, "https://example.com") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecShellRequiresDocker(t *testing.T) {
	// Hide docker binary via PATH so the test never spawns containers.
	t.Setenv("PATH", t.TempDir())
	if _, err := ExecShell(context.Background(), "alpine:latest", "echo hi", 5); err == nil {
		t.Fatal("expected error when docker is unavailable")
	}
}
