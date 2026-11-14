package memory

import (
	"strings"
	"testing"

	"dis-llm-bot/internal/store"
)

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
