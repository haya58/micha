package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"

	"dis-llm-bot/internal/config"
)

func TestIsMentionOfDirect(t *testing.T) {
	if !isMentionOf("123", "hey <@123> hello", nil) {
		t.Fatal("should detect <@id> mention")
	}
	if !isMentionOf("123", "hey <@!123> hello", nil) {
		t.Fatal("should detect <@!id> mention")
	}
	if isMentionOf("123", "hey <@456> hello", nil) {
		t.Fatal("should not match other user")
	}
}

func TestIsMentionOfViaList(t *testing.T) {
	mentions := []*discordgo.User{{ID: "123"}}
	if !isMentionOf("123", "hey", mentions) {
		t.Fatal("should detect mention via list")
	}
	if isMentionOf("999", "hey", mentions) {
		t.Fatal("should not match missing id")
	}
}

func TestAllowedCategory(t *testing.T) {
	b := &Bot{cfg: &config.Config{CategoryIDs: []string{"cat1", "cat2"}}}
	if !b.allowedCategory("cat1") || !b.allowedCategory("cat2") {
		t.Fatal("allowed categories not matched")
	}
	if b.allowedCategory("other") {
		t.Fatal("unknown category must not match")
	}
}

func TestStatusTextForTool(t *testing.T) {
	title, desc := statusTextForTool("brave_search", `{"query":"IIJ 評判","count":5}`)
	if title != "Web検索中……" || desc != "IIJ 評判" {
		t.Fatalf("got %q %q", title, desc)
	}
	title, desc = statusTextForTool("fetch_page", `{"url":"https://example.com/a"}`)
	if title != "ページ取得中……" || desc != "https://example.com/a" {
		t.Fatalf("got %q %q", title, desc)
	}
	title, desc = statusTextForTool("exec_shell", `{"command":"ls -la"}`)
	if title != "コマンド実行中……" || desc != "ls -la" {
		t.Fatalf("got %q %q", title, desc)
	}
	title, _ = statusTextForTool("mystery_tool", `{}`)
	if title != "処理中……" {
		t.Fatalf("got %q", title)
	}
	// malformed args must not crash
	statusTextForTool("brave_search", "not json")
}

func TestNilStatusNoop(t *testing.T) {
	var st *Status
	st.Update("a", "b") // must not panic
	st.Done()           // must not panic
}

func TestStripSelfPrefix(t *testing.T) {
	b := &Bot{
		cfg:     &config.Config{DisplayName: "my-bot"},
		botID:   "123",
		botName: "dis-llm-bot",
	}
	cases := map[string]string{
		"dis-llm-bot: こんにちは":       "こんにちは",
		"dls-llm-bot： やっほー":        "やっほー",
		"my-bot: hello":            "hello",
		"<@123> こんにちは":             "こんにちは",
		"<@!123> こんにちは":            "こんにちは",
		"dis-llm-bot: my-bot: ネスト": "ネスト",
		"結論：これは残る":                 "結論：これは残る",
		"ただの本文":                    "ただの本文",
		"dis-llm-bot":              "dis-llm-bot", // no separator: untouched
	}
	for in, want := range cases {
		if got := b.stripSelfPrefix(in); got != want {
			t.Fatalf("stripSelfPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
