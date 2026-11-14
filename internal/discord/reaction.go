package discord

import (
	"context"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"dis-llm-bot/internal/llm"
)

// reactionEmojis maps LLM-facing aliases to unicode emoji.
// The picker model only ever sees the aliases.
var reactionEmojis = map[string]string{
	"thumbsup":    "👍",
	"thumbsdown":  "👎",
	"heart":       "❤️",
	"laugh":       "😂",
	"smile":       "😄",
	"cry":         "😭",
	"surprise":    "😮",
	"thinking":    "🤔",
	"tada":        "🎉",
	"eyes":        "👀",
	"fire":        "🔥",
	"pray":        "🙏",
	"ok_hand":     "👌",
	"clap":        "👏",
	"wave":        "👋",
	"star":        "⭐",
	"check":       "✅",
	"question":    "❓",
	"exclamation": "❗",
	"coffee":      "☕",
	"moon":        "🌙",
	"sleep":       "😴",
}

// maybeReact adds at most one emoji reaction to the trigger message.
// Probability + per-channel cooldown keep it moderate. All failures are
// swallowed (logged only) so reactions never break replies.
func (b *Bot) maybeReact(m *discordgo.MessageCreate) {
	if b.cfg.Reaction.Probability <= 0 {
		return
	}
	if strings.TrimSpace(m.Content) == "" && len(m.Attachments) == 0 {
		return
	}
	b.reactMu.Lock()
	cooled := time.Since(b.lastReact[m.ChannelID]) < time.Duration(b.cfg.Reaction.CooldownSec)*time.Second
	if !cooled {
		b.lastReact[m.ChannelID] = time.Now()
	}
	b.reactMu.Unlock()
	if cooled {
		return
	}
	if rand.Float64() > b.cfg.Reaction.Probability {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	alias := b.pickReaction(ctx, m)
	if alias == "" {
		return
	}
	if err := b.session.MessageReactionAdd(m.ChannelID, m.ID, reactionEmojis[alias]); err != nil {
		log.Printf("[react] add %s failed: %v", alias, err)
	}
}

// pickReaction asks the LLM to choose one alias from reactionEmojis for the
// message, or NONE. Returns "" when no reaction fits or the call fails.
func (b *Bot) pickReaction(ctx context.Context, m *discordgo.MessageCreate) string {
	aliases := make([]string, 0, len(reactionEmojis))
	for a := range reactionEmojis {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	out, err := b.llm.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: "You pick one Discord reaction emoji for a chat message. Reply with ONLY one alias from this list, or NONE if no reaction fits: " + strings.Join(aliases, ", ")},
		{Role: "user", Content: m.Author.Username + ": " + m.Content},
	}, nil)
	if err != nil {
		log.Printf("[react] pick failed: %v", err)
		return ""
	}
	return parseReactionChoice(out.Content)
}

// parseReactionChoice validates the model's answer against the alias table.
func parseReactionChoice(s string) string {
	fields := strings.Fields(strings.ToLower(s))
	if len(fields) == 0 {
		return ""
	}
	if _, ok := reactionEmojis[fields[0]]; ok {
		return fields[0]
	}
	return ""
}
