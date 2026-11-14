package discord

import (
	"encoding/json"
	"sync"

	"github.com/bwmarrin/discordgo"

	"dis-llm-bot/internal/textutil"
)

// Status is a temporary "working..." embed message posted while a long
// operation runs. It can be updated in place as phases change and is
// deleted when the work finishes.
//
// All methods are safe on a nil Status (no-op), so callers can ignore
// post errors (e.g. missing permissions) and keep the same code path.
type Status struct {
	mu        sync.Mutex
	session   *discordgo.Session
	channelID string
	msgID     string
}

// postStatus publishes the initial working embed. Returns nil when it
// cannot be posted; callers should proceed without status updates.
func postStatus(s *discordgo.Session, channelID, title, desc string) *Status {
	msg, err := s.ChannelMessageSendEmbed(channelID, statusEmbed(title, desc))
	if err != nil || msg == nil {
		return nil
	}
	return &Status{session: s, channelID: channelID, msgID: msg.ID}
}

func statusEmbed(title, desc string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0x5865F2, // blurple: "bot is working"
	}
}

// Update replaces the embed text in place. Concurrent calls are serialized.
func (st *Status) Update(title, desc string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	_, _ = st.session.ChannelMessageEditEmbed(st.channelID, st.msgID, statusEmbed(title, desc))
}

// Done removes the status message. Errors are ignored.
func (st *Status) Done() {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	_ = st.session.ChannelMessageDelete(st.channelID, st.msgID)
}

// statusTextForTool maps a tool call to a user-facing progress message.
// Pure function so it can be unit tested without a Discord session.
func statusTextForTool(name, argsJSON string) (title, desc string) {
	var args map[string]any
	_ = json.Unmarshal([]byte(argsJSON), &args)
	str := func(key string) string {
		s, _ := args[key].(string)
		return textutil.Truncate(s, 100)
	}
	switch name {
	case "brave_search":
		return "Web検索中……", str("query")
	case "fetch_page":
		return "ページ取得中……", str("url")
	case "exec_shell":
		return "コマンド実行中……", str("command")
	default:
		return "処理中……", ""
	}
}
