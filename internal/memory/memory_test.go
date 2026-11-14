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

func TestMergeNewest(t *testing.T) {
	a := []store.Message{{ID: 1}, {ID: 5}, {ID: 9}}
	b := []store.Message{{ID: 2}, {ID: 7}}
	got := MergeNewest([][]store.Message{a, b, nil}, 0)
	want := []int64{9, 7, 5, 2, 1}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if got := MergeNewest([][]store.Message{a, b}, 3); len(got) != 3 || got[0].ID != 9 {
		t.Fatalf("limit not applied: %v", got)
	}
	if got := MergeNewest(nil, 10); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}
