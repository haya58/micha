package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"dis-llm-bot/internal/llm"
	"dis-llm-bot/internal/store"
	"dis-llm-bot/internal/textutil"
)

// Context bundles history + relevant long-term memories for prompt building.
type Context struct {
	History       []store.Message
	Memories      []store.Memory
	Readonly      []store.Message
	ReadonlyNames map[string]string // channelID -> display name
}

// Build gathers recent messages and (best-effort) vector memories.
// Embedding failures must not break replies, so errors return history only.
func Build(ctx context.Context, st *store.Store, lc *llm.Client, guildID, channelID string, threadID *string, limit, topK int, query string) Context {
	hist, _ := st.RecentByScope(ctx, guildID, channelID, threadID, limit)
	c := Context{History: hist}
	if lc == nil || st == nil || strings.TrimSpace(query) == "" || topK <= 0 {
		return c
	}
	embs, err := lc.Embed(ctx, []string{query})
	if err != nil || len(embs) == 0 {
		return c
	}
	mems, err := st.SearchMemories(ctx, guildID, embs[0], topK)
	if err != nil {
		return c
	}
	c.Memories = mems
	return c
}

// MergeNewest merges per-channel histories (each oldest-first) into one
// newest-first list capped at limit. Used to give every read-only channel
// a fair share of context instead of one busy channel crowding out the rest.
func MergeNewest(lists [][]store.Message, limit int) []store.Message {
	var all []store.Message
	for _, l := range lists {
		all = append(all, l...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

func FormatPrompt(mc Context) string {
	var b strings.Builder
	if len(mc.Memories) > 0 {
		b.WriteString("Known facts about users/server:\n")
		for _, m := range mc.Memories {
			fmt.Fprintf(&b, "- [%s] %s\n", m.Kind, m.Summary)
		}
		// Each summary is prefixed with the user who stated it ("name: ...").
		// Treat them as shared server knowledge both when recalling and replying.
		b.WriteString("Each fact is prefixed with the user who stated it.\n\n")
	}
	if len(mc.Readonly) > 0 {
		b.WriteString("Recent topics in read-only channels (for background only, do not reply there):\n")
		for _, m := range mc.Readonly {
			name := mc.ReadonlyNames[m.ChannelID]
			if name == "" {
				name = m.ChannelID
			}
			fmt.Fprintf(&b, "- [%s][%s]: %s\n", name, m.Username, textutil.Truncate(m.Content, 200))
		}
		b.WriteString("\n")
	}
	return b.String()
}
