package memory

import (
	"strings"
	"testing"

	"dis-llm-bot/internal/store"
)

func TestShouldRememberShort(t *testing.T) {
	if _, _, ok := ShouldRemember("hi"); ok {
		t.Fatal("short message should not be remembered")
	}
}

func TestShouldRememberLong(t *testing.T) {
	content := "私はラーメンが好きで、特に味噌ラーメンをよく食べます。毎週末に新しい店を開拓しています。"
	summary, imp, ok := ShouldRemember(content)
	if !ok {
		t.Fatal("long message should be remembered")
	}
	if summary == "" || imp <= 0 {
		t.Fatalf("summary=%q imp=%d", summary, imp)
	}
}

func TestShouldRememberLinkOnly(t *testing.T) {
	// A bare link share (typical tweet URL, 60+ chars) must not trigger
	// memorization: URLs don't count toward the length threshold.
	if _, _, ok := ShouldRemember("これ見て https://x.com/someuser/status/1234567890123456789"); ok {
		t.Fatal("bare link share should not be remembered")
	}
	if _, _, ok := ShouldRemember("https://x.com/someuser/status/1234567890123456789"); ok {
		t.Fatal("URL alone should not be remembered")
	}
	// Long personal text alongside a link is still remembered (URL kept).
	content := "先週話した味噌ラーメンの店に昨日も行ってきたよ。やっぱりあの濃厚なスープは最高だったのでまた行きたい https://example.com/ramen-shop-review"
	summary, _, ok := ShouldRemember(content)
	if !ok {
		t.Fatal("substantive message with a link should be remembered")
	}
	if !strings.Contains(summary, "味噌ラーメン") {
		t.Fatalf("summary lost content: %q", summary)
	}
}

func TestFormatPrompt(t *testing.T) {
	mc := Context{
		Memories: []store.Memory{
			{Kind: "fact", Summary: "haya: likes miso ramen"},
			{Kind: "preference", Summary: "taro: speaks Japanese"},
		},
	}
	out := FormatPrompt(mc)
	for _, want := range []string{"likes miso ramen", "speaks Japanese", "fact", "preference"} {
		if !strings.Contains(out, want) {
			t.Fatalf("FormatPrompt missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "prefixed with the user who stated it") {
		t.Fatalf("FormatPrompt must carry attribution guidance: %q", out)
	}
	if out := FormatPrompt(Context{}); out != "" {
		t.Fatalf("empty memories should yield empty prompt, got %q", out)
	}
}

func TestFormatPromptReadonly(t *testing.T) {
	mc := Context{
		Readonly: []store.Message{
			{ChannelID: "ch1", Username: "u1", Content: "新製品の話"},
		},
		ReadonlyNames: map[string]string{"ch1": "#announce"},
	}
	out := FormatPrompt(mc)
	for _, want := range []string{"#announce", "u1", "新製品の話", "read-only"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
