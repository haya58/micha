package discord

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"dis-llm-bot/internal/textutil"
)

// Status is a temporary "working..." embed message posted while a long
// operation runs. It can be updated in place as phases change and is
// deleted when the work finishes.
//
// The embed is posted lazily: only if the work is still running when the
// delay elapses. Fast replies never show a status message at all.
//
// All methods are safe on a nil Status (no-op), so callers can ignore
// post errors (e.g. missing permissions) and keep the same code path.
type Status struct {
	mu        sync.Mutex
	session   *discordgo.Session
	channelID string
	msgID     string
	title     string
	desc      string
	posted    bool
	dismissed bool
	timer     *time.Timer
}

// statusDelay is the fixed threshold: the working embed appears only if
// the reply is still running after this long.
const statusDelay = 3 * time.Second

// postStatusDelayed returns a Status that publishes its embed only if the
// work outlasts delay. Call Done when the work finishes.
func postStatusDelayed(s *discordgo.Session, channelID string, delay time.Duration, title, desc string) *Status {
	st := &Status{session: s, channelID: channelID, title: title, desc: desc}
	st.timer = time.AfterFunc(delay, st.postPending)
	return st
}

func (st *Status) postPending() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.posted || st.dismissed {
		return
	}
	msg, err := st.session.ChannelMessageSendEmbed(st.channelID, statusEmbed(st.title, st.desc))
	if err != nil || msg == nil {
		st.dismissed = true // give up silently; the reply itself still goes out
		return
	}
	st.msgID = msg.ID
	st.posted = true
}

func statusEmbed(title, desc string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0x5865F2, // blurple: "bot is working"
	}
}

// Update replaces the embed text in place. Concurrent calls are serialized.
// Before the embed is posted, it only refreshes the pending text.
func (st *Status) Update(title, desc string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.title, st.desc = title, desc
	if !st.posted {
		return
	}
	_, _ = st.session.ChannelMessageEditEmbed(st.channelID, st.msgID, statusEmbed(title, desc))
}

// Done removes the status message, or cancels it if it was never posted.
// Errors are ignored.
func (st *Status) Done() {
	if st == nil {
		return
	}
	if st.timer != nil {
		st.timer.Stop()
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.dismissed = true
	if !st.posted {
		return
	}
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
