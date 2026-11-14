package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"dis-llm-bot/internal/config"
	"dis-llm-bot/internal/llm"
	"dis-llm-bot/internal/memory"
	"dis-llm-bot/internal/store"
	"dis-llm-bot/internal/textutil"
	"dis-llm-bot/internal/tools"
)

// Bot wires Discord events to the LLM + store.
type Bot struct {
	cfg     *config.Config
	session *discordgo.Session
	store   *store.Store
	llm     *llm.Client
	persona string
	botID   string
}

func New(cfg *config.Config, st *store.Store, lc *llm.Client, persona string) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentMessageContent
	b := &Bot{cfg: cfg, session: s, store: st, llm: lc, persona: persona}
	s.AddHandler(b.onMessage)
	s.AddHandler(b.onReady)
	return b, nil
}

func (b *Bot) Open() error  { return b.session.Open() }
func (b *Bot) Close() error { return b.session.Close() }

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.botID = r.User.ID
}

// scope resolves channel/thread and whether the message is in an allowed category.
func (b *Bot) scope(m *discordgo.MessageCreate) (channelID string, threadID *string, ok bool) {
	if m.GuildID == "" || m.GuildID != b.cfg.GuildID {
		return "", nil, false
	}
	ch, err := b.session.Channel(m.ChannelID)
	if err != nil {
		return "", nil, false
	}
	if ch.IsThread() {
		parentID := ch.ParentID
		if parentID == "" {
			parentID = ch.ID
		}
		parent, err := b.session.Channel(parentID)
		if err != nil {
			return "", nil, false
		}
		if !b.allowedCategory(parent.ParentID) {
			return "", nil, false
		}
		t := m.ChannelID
		return parentID, &t, true
	}
	if !b.allowedCategory(ch.ParentID) {
		return "", nil, false
	}
	return m.ChannelID, nil, true
}

func (b *Bot) allowedCategory(parentID string) bool {
	for _, c := range b.cfg.CategoryIDs {
		if c == parentID {
			return true
		}
	}
	return false
}

func (b *Bot) hasPermission(guildID, channelID string) bool {
	perms, err := b.session.UserChannelPermissions(b.botID, channelID)
	if err != nil {
		return false
	}
	return perms&discordgo.PermissionViewChannel != 0 &&
		perms&discordgo.PermissionSendMessages != 0 &&
		perms&discordgo.PermissionReadMessageHistory != 0
}

func isMentionOf(botID, content string, mentions []*discordgo.User) bool {
	if strings.Contains(content, "<@"+botID+">") || strings.Contains(content, "<@!"+botID+">") {
		return true
	}
	for _, u := range mentions {
		if u.ID == botID {
			return true
		}
	}
	return false
}

func (b *Bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	if b.botID == "" {
		me, err := s.User("@me")
		if err == nil {
			b.botID = me.ID
		}
	}
	channelID, threadID, ok := b.scope(m)
	if !ok {
		return
	}
	if !b.hasPermission(m.GuildID, m.ChannelID) {
		return
	}
	mentioned := isMentionOf(b.botID, m.Content, m.Mentions)
	isReply := m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil && m.ReferencedMessage.Author.ID == b.botID
	if !mentioned && !isReply {
		return // non-trigger messages are handled by the random scheduler
	}
	b.handleReply(m, channelID, threadID)
}

// threadStr renders an optional thread ID for logs.
func threadStr(threadID *string) string {
	if threadID == nil || *threadID == "" {
		return "-"
	}
	return *threadID
}

func (b *Bot) handleReply(m *discordgo.MessageCreate, channelID string, threadID *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	log.Printf("[reply] trigger guild=%s ch=%s thread=%s user=%s msg=%.80q",
		m.GuildID, channelID, threadStr(threadID), m.Author.Username, m.Content)

	attachmentsJSON, _ := json.Marshal(m.Attachments)
	stMsg := store.Message{
		GuildID: m.GuildID, ChannelID: channelID, ThreadID: threadID,
		UserID: m.Author.ID, Username: m.Author.Username,
		Content: m.Content, Attachments: string(attachmentsJSON),
	}
	msgID, err := b.store.SaveMessage(ctx, stMsg)
	if err != nil {
		log.Printf("[reply] SaveMessage failed: %v", err)
	}

	// Enrich user content with image/link context before LLM call.
	enriched := b.enrichContent(ctx, m)

	mctx := memory.Build(ctx, b.store, b.llm, m.GuildID, channelID, threadID, b.cfg.HistoryLimit, b.cfg.MemoryTopK, m.Content)

	system := b.persona + "\n\nServer time: " + time.Now().Format(time.RFC3339) + "\n" + memory.FormatPrompt(mctx)
	var msgs []llm.ChatMessage
	msgs = append(msgs, llm.ChatMessage{Role: "system", Content: system})
	for _, h := range mctx.History {
		role := "user"
		if h.UserID == b.botID {
			role = "assistant"
		}
		msgs = append(msgs, llm.ChatMessage{Role: role, Content: h.Username + ": " + h.Content})
	}
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: m.Author.Username + ": " + enriched})

	reply, toolErr := b.chatWithTools(ctx, msgs, msgID)
	if toolErr != nil {
		log.Printf("[reply] chat failed guild=%s ch=%s thread=%s msgID=%d hist=%d mem=%d: %v",
			m.GuildID, channelID, threadStr(threadID), msgID, len(mctx.History), len(mctx.Memories), toolErr)
		reply = "ごめん、今うまく考えられなかった。もう一度言ってみて。"
	}
	if err := b.sendSplit(m.ChannelID, reply); err != nil {
		log.Printf("[reply] send failed ch=%s msgID=%d: %v", m.ChannelID, msgID, err)
		return
	}

	if _, err := b.store.SaveMessage(ctx, store.Message{
		GuildID: m.GuildID, ChannelID: channelID, ThreadID: threadID,
		UserID: b.botID, Username: b.cfg.DisplayName, Content: reply,
		Attachments: "[]",
	}); err != nil {
		log.Printf("[reply] SaveMessage(assistant) failed: %v", err)
	}
	// Prototype long-term memory: store notable user messages with embeddings.
	if summary, imp, ok := memory.ShouldRemember(m.Content); ok {
		embs, err := b.llm.Embed(ctx, []string{m.Content})
		if err != nil {
			log.Printf("[reply] embed failed (memory skipped): %v", err)
		} else if len(embs) > 0 {
			scope := channelID
			if threadID != nil {
				scope = *threadID
			}
			if err := b.store.SaveMemory(ctx, m.GuildID, scope, "fact", m.Author.Username+": "+summary, embs[0], imp); err != nil {
				log.Printf("[reply] SaveMemory failed: %v", err)
			}
		}
	}
}

func (b *Bot) chatWithTools(ctx context.Context, msgs []llm.ChatMessage, msgID int64) (string, error) {
	toolDefs := llm.DefaultTools(b.cfg.ShellEnabled)
	cur := append([]llm.ChatMessage{}, msgs...)
	for step := 0; step < 3; step++ {
		out, err := b.llm.Chat(ctx, cur, toolDefs)
		if err != nil {
			log.Printf("[llm] chat step=%d msgID=%d failed: %v", step, msgID, err)
			return "", err
		}
		if len(out.ToolCalls) == 0 {
			log.Printf("[llm] chat step=%d msgID=%d done replyLen=%d", step, msgID, len(out.Content))
			return out.Content, nil
		}
		names := make([]string, 0, len(out.ToolCalls))
		for _, tc := range out.ToolCalls {
			names = append(names, tc.Function.Name)
		}
		log.Printf("[llm] chat step=%d msgID=%d toolCalls=%v", step, msgID, names)
		cur = append(cur, out)
		// Run independent tool calls concurrently (results keep call order).
		toolMsgs := make([]llm.ChatMessage, len(out.ToolCalls))
		var wg sync.WaitGroup
		for i, tc := range out.ToolCalls {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := b.runTool(ctx, tc, msgID)
				toolMsgs[i] = llm.ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: res}
			}()
		}
		wg.Wait()
		cur = append(cur, toolMsgs...)
	}
	// final answer without tools
	out, err := b.llm.Chat(ctx, cur, nil)
	if err != nil {
		return "", err
	}
	return out.Content, nil
}

func (b *Bot) runTool(ctx context.Context, tc llm.ToolCall, msgID int64) string {
	start := time.Now()
	var args map[string]any
	_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
	var result string
	switch tc.Function.Name {
	case "brave_search":
		q, _ := args["query"].(string)
		n := 5
		if f, ok := args["count"].(float64); ok {
			n = int(f)
		}
		r, err := tools.BraveSearch(ctx, b.cfg.BraveAPIKey, q, n)
		if err != nil {
			result = "search error: " + err.Error()
		} else {
			result = r
		}
	case "fetch_page":
		u, _ := args["url"].(string)
		r, err := tools.FetchPage(ctx, u)
		if err != nil {
			result = "fetch error: " + err.Error()
		} else {
			result = r
		}
	case "exec_shell":
		if !b.cfg.ShellEnabled {
			result = "shell is disabled"
			break
		}
		cmdStr, _ := args["command"].(string)
		to := b.cfg.ShellTimeoutSec
		if f, ok := args["timeout"].(float64); ok && f > 0 {
			to = int(f)
		}
		r, err := tools.ExecShell(ctx, b.cfg.ShellImage, cmdStr, to)
		if err != nil {
			result = "shell error: " + err.Error()
		} else {
			result = r
		}
	default:
		result = "unknown tool: " + tc.Function.Name
	}
	if err := b.store.SaveToolRun(ctx, &msgID, tc.Function.Name, tc.Function.Arguments, result, int(time.Since(start).Milliseconds())); err != nil {
		log.Printf("[tool] SaveToolRun failed: %v", err)
	}
	snip := textutil.Truncate(result, 300)
	log.Printf("[tool] %s msgID=%d latency=%dms err=%v result=%.300q",
		tc.Function.Name, msgID, int(time.Since(start).Milliseconds()),
		isToolError(result), snip)
	return result
}

// isToolError reports whether a tool result string is one of our error shapes.
func isToolError(result string) bool {
	for _, p := range []string{"search error:", "fetch error:", "shell error:", "unknown tool:"} {
		if strings.HasPrefix(result, p) {
			return true
		}
	}
	return false
}

// enrichContent downloads image attachments (for vision notes) and pre-fetches
// the first URL in the message so the LLM has concrete material.
func (b *Bot) enrichContent(ctx context.Context, m *discordgo.MessageCreate) string {
	content := m.Content
	// image attachments: download to verify + note for the model
	for _, a := range m.Attachments {
		if strings.HasPrefix(a.ContentType, "image/") {
			if data, ct, err := downloadBytes(ctx, a.URL); err == nil {
				content += fmt.Sprintf("\n[attached image %s (%s, %d bytes) — user shared this image; describe/react to it]", a.Filename, ct, len(data))
			} else {
				log.Printf("[enrich] image download failed %s: %v", a.URL, err)
				content += fmt.Sprintf("\n[attached image %s could not be downloaded: %v]", a.Filename, err)
			}
		}
	}
	// first http URL in content: fetch text eagerly (tool remains for follow-ups)
	if u := firstURL(content); u != "" {
		if txt, err := tools.FetchPage(ctx, u); err == nil {
			content += "\n\n[linked page content]\n" + textutil.Truncate(txt, 3000)
		} else {
			log.Printf("[enrich] link prefetch failed %s: %v", u, err)
		}
	}
	return content
}

func firstURL(s string) string {
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			return strings.Trim(f, "<>.,)\"'")
		}
	}
	return ""
}

func downloadBytes(ctx context.Context, url string) ([]byte, string, error) {
	ctx2, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx2, "GET", url, nil)
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return b, resp.Header.Get("Content-Type"), nil
}

func (b *Bot) sendSplit(channelID, text string) error {
	if text == "" {
		text = "(empty reply)"
	}
	_ = b.session.ChannelTyping(channelID)
	for len(text) > 0 {
		chunk := text
		if len([]rune(chunk)) > 1900 {
			runes := []rune(chunk)
			chunk = string(runes[:1900])
			text = string(runes[1900:])
		} else {
			text = ""
		}
		if _, err := b.session.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// ChannelSnapshot is used by the random-talk scheduler.
type ChannelSnapshot struct {
	ChannelID string
	ThreadID  *string
	GuildID   string
	LastMsgID string
}

// ActiveChannels lists text channels under allowed categories for random talk.
func (b *Bot) ActiveChannels() ([]ChannelSnapshot, error) {
	allowed := map[string]bool{}
	for _, c := range b.cfg.CategoryIDs {
		allowed[c] = true
	}
	chs, err := b.session.GuildChannels(b.cfg.GuildID)
	if err != nil {
		return nil, err
	}
	var out []ChannelSnapshot
	for _, ch := range chs {
		if ch.Type == discordgo.ChannelTypeGuildText && allowed[ch.ParentID] {
			out = append(out, ChannelSnapshot{ChannelID: ch.ID, GuildID: b.cfg.GuildID, LastMsgID: ch.LastMessageID})
		}
	}
	return out, nil
}

func (b *Bot) RandomSay(channelID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	hist, _ := b.store.RecentByScope(ctx, b.cfg.GuildID, channelID, nil, 8)
	var msgs []llm.ChatMessage
	msgs = append(msgs, llm.ChatMessage{Role: "system", Content: b.persona + "\nYou may start a light casual conversation. Keep it short. If nothing interesting comes to mind, reply with exactly: SKIP"})
	for _, h := range hist {
		role := "user"
		if h.UserID == b.botID {
			role = "assistant"
		}
		msgs = append(msgs, llm.ChatMessage{Role: role, Content: h.Username + ": " + h.Content})
	}
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: "何か一言どうぞ（つまらなければSKIPとだけ返して）"})
	out, err := b.llm.Chat(ctx, msgs, nil)
	if err != nil || strings.TrimSpace(out.Content) == "" || strings.TrimSpace(out.Content) == "SKIP" {
		return
	}
	if !b.hasPermission(b.cfg.GuildID, channelID) {
		return
	}
	_ = b.sendSplit(channelID, out.Content)
	_, _ = b.store.SaveMessage(ctx, store.Message{
		GuildID: b.cfg.GuildID, ChannelID: channelID,
		UserID: b.botID, Username: b.cfg.DisplayName, Content: out.Content, Attachments: "[]",
	})
}
