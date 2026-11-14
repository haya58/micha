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

func TestTweetStatusID(t *testing.T) {
	cases := map[string]string{
		"https://x.com/user/status/123456":         "123456",
		"https://twitter.com/user/status/123456":   "123456",
		"http://www.x.com/u/status/987":            "987",
		"https://x.com/user/status/123456/photo/1": "123456",
		"https://x.com/user/statuses":              "",
		"https://example.com/a/status/123":         "",
		"https://x.com/i/status/123":               "123",
		"not a url":                                "",
	}
	for in, want := range cases {
		if got := TweetStatusID(in); got != want {
			t.Fatalf("TweetStatusID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchTweet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/i/status/123" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"tweet":{"text":"hello pic","author":{"name":"Taro","screen_name":"taro"},"media":{"photos":[{"url":"https://pbs.example/a.jpg"},{"url":"https://pbs.example/b.jpg"}]}}}`))
	}))
	defer srv.Close()

	old := fxtwitterBaseURL
	fxtwitterBaseURL = srv.URL
	defer func() { fxtwitterBaseURL = old }()

	text, imgs, err := FetchTweet(context.Background(), "https://x.com/taro/status/123")
	if err != nil {
		t.Fatalf("FetchTweet: %v", err)
	}
	if !strings.Contains(text, "hello pic") || !strings.Contains(text, "@taro") {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(imgs) != 2 || imgs[0] != "https://pbs.example/a.jpg" {
		t.Fatalf("unexpected images: %v", imgs)
	}

	if _, _, err := FetchTweet(context.Background(), "https://x.com/taro/status/999"); err == nil {
		t.Fatal("expected error for missing tweet")
	}
	if _, _, err := FetchTweet(context.Background(), "https://example.com/"); err == nil {
		t.Fatal("expected error for non-tweet URL")
	}
}

func TestFetchPageRoutesTweets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"tweet":{"text":"via tool","media":{"photos":[{"url":"https://pbs.example/c.jpg"}]}}}`))
	}))
	defer srv.Close()

	old := fxtwitterBaseURL
	fxtwitterBaseURL = srv.URL
	defer func() { fxtwitterBaseURL = old }()

	out, err := FetchPage(context.Background(), "https://twitter.com/u/status/42")
	if err != nil {
		t.Fatalf("FetchPage tweet: %v", err)
	}
	if !strings.Contains(out, "via tool") || !strings.Contains(out, "https://pbs.example/c.jpg") {
		t.Fatalf("unexpected output: %q", out)
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
