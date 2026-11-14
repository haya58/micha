package memory

import (
	"context"
	"fmt"
	"strings"

	"dis-llm-bot/internal/llm"
	"dis-llm-bot/internal/store"
	"dis-llm-bot/internal/textutil"
)

// Context bundles history + relevant long-term memories for prompt building.
type Context struct {
	History  []store.Message
	Memories []store.Memory
}

// Build gathers recent messages and (best-effort) vector memories.
// userID scopes memory recall to the querying user first; general
// (non-user-specific) memories still follow by similarity.
// Embedding failures must not break replies, so errors return history only.
func Build(ctx context.Context, st *store.Store, lc *llm.Client, guildID, userID, channelID string, threadID *string, limit, topK int, query string) Context {
	hist, _ := st.RecentByScope(ctx, guildID, channelID, threadID, limit)
	c := Context{History: hist}
	if lc == nil || st == nil || strings.TrimSpace(query) == "" || topK <= 0 {
		return c
	}
	embs, err := lc.Embed(ctx, []string{query})
	if err != nil || len(embs) == 0 {
		return c
	}
	mems, err := st.SearchMemories(ctx, guildID, userID, embs[0], topK)
	if err != nil {
		return c
	}
	c.Memories = mems
	return c
}

func FormatPrompt(mc Context) string {
	var b strings.Builder
	if len(mc.Memories) > 0 {
		b.WriteString("Known facts about users/server:\n")
		for _, m := range mc.Memories {
			fmt.Fprintf(&b, "- [%s] %s\n", m.Kind, m.Summary)
		}
		// Each summary is prefixed with the user who stated it ("name: ...").
		// A user-specific preference/instruction applies only to that user,
		// never to other users asking similar things.
		b.WriteString("User-specific preferences apply only to the user who stated them.\n\n")
	}
	return b.String()
}

// ShouldRemember is a cheap heuristic for the prototype: remember longer user
// messages (TomoriBot-style summarization is a future step; here we store raw
// excerpts with importance 1-2 so recall works without an extra LLM call).
func ShouldRemember(content string) (string, int, bool) {
	t := strings.TrimSpace(content)
	if len([]rune(t)) < 40 {
		return "", 0, false
	}
	return textutil.Truncate(t, 500), 1, true
}
