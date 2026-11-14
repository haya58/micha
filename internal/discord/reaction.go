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

// reactionEmojis is the built-in unicode fallback table (alias -> emoji).
// The picker model only ever sees aliases.
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

// emojiCacheTTL bounds how long guild emojis are cached.
const emojiCacheTTL = time.Hour

// customEmojiMap converts guild custom emojis to alias -> reaction ID.
// Custom emojis take precedence over same-named unicode aliases.
func customEmojiMap(emojis []*discordgo.Emoji, base map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(emojis))
	for k, v := range base {
		merged[k] = v
	}
	for _, e := range emojis {
		if e.Name == "" || e.ID == "" {
			continue
		}
		if e.Animated {
			merged[e.Name] = "a:" + e.Name + ":" + e.ID
		} else {
			merged[e.Name] = e.Name + ":" + e.ID
		}
	}
	return merged
}

// emojiMap returns all server-available reaction aliases: guild custom
// emojis first, unicode fallback second. Cached for emojiCacheTTL.
func (b *Bot) emojiMap() map[string]string {
	b.emojiMu.Lock()
	defer b.emojiMu.Unlock()
	if b.emojiCache != nil && time.Since(b.emojiFetched) < emojiCacheTTL {
		return b.emojiCache
	}
	table := reactionEmojis
	if emojis, err := b.session.GuildEmojis(b.cfg.Discord.GuildID); err != nil {
		log.Printf("[react] guild emojis failed, using unicode only: %v", err)
	} else {
		table = customEmojiMap(emojis, reactionEmojis)
		log.Printf("[react] %d custom emojis available", len(table)-len(reactionEmojis))
	}
	b.emojiCache = table
	b.emojiFetched = time.Now()
	return table
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
	alias, table := b.pickReaction(ctx, m)
	if alias == "" {
		return
	}
	if err := b.session.MessageReactionAdd(m.ChannelID, m.ID, table[alias]); err != nil {
		log.Printf("[react] add %s failed: %v", alias, err)
	}
}

// pickReaction asks the LLM to choose one alias from the server-available
// table for the message, or NONE. Returns the alias and the table used.
func (b *Bot) pickReaction(ctx context.Context, m *discordgo.MessageCreate) (string, map[string]string) {
	table := b.emojiMap()
	aliases := make([]string, 0, len(table))
	for a := range table {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	out, err := b.llm.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: "You pick one Discord reaction emoji for a chat message. Reply with ONLY one alias from this list, or NONE if no reaction fits: " + strings.Join(aliases, ", ")},
		{Role: "user", Content: m.Author.Username + ": " + m.Content},
	}, nil)
	if err != nil {
		log.Printf("[react] pick failed: %v", err)
		return "", table
	}
	return parseReactionChoice(out.Content, table), table
}

// parseReactionChoice validates the model's answer against the alias table.
func parseReactionChoice(s string, table map[string]string) string {
	fields := strings.Fields(strings.ToLower(s))
	if len(fields) == 0 {
		return ""
	}
	if _, ok := table[fields[0]]; ok {
		return fields[0]
	}
	return ""
}
