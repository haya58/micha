package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	cfg          *config.Config
	session      *discordgo.Session
	store        *store.Store
	llm          *llm.Client
	persona      string
	botID        string
	botName      string
	readyOnce    sync.Once
	reactMu      sync.Mutex
	lastReact    map[string]time.Time
	emojiMu      sync.Mutex
	emojiCache   map[string]string
	emojiFetched time.Time
	chNamesMu    sync.Mutex
	chNames      map[string]string
}

func New(cfg *config.Config, st *store.Store, lc *llm.Client, persona string) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, err
	}
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentMessageContent
	b := &Bot{cfg: cfg, session: s, store: st, llm: lc, persona: persona, chNames: map[string]string{}, lastReact: map[string]time.Time{}}
	s.AddHandler(b.onMessage)
	s.AddHandler(b.onReady)
	return b, nil
}

func (b *Bot) Open() error  { return b.session.Open() }
func (b *Bot) Close() error { return b.session.Close() }

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.botID = r.User.ID
	b.botName = r.User.Username
	// onReady fires on every reconnect; announce only once per process.
	b.readyOnce.Do(func() { b.announceLogin() })
}

// announceLogin posts a "logged in" embed. Best-effort: failures are logged.
func (b *Bot) announceLogin() {
	channelID := b.cfg.Discord.LoginNoticeChannelID
	if channelID == "" {
		chs, err := b.ActiveChannels()
		if err != nil {
			log.Printf("[ready] ActiveChannels failed: %v", err)
			return
		}
		if len(chs) == 0 {
			log.Printf("[ready] no text channel under allowed categories")
			return
		}
		channelID = chs[0].ChannelID
	}
	if !b.hasPermission(b.cfg.Discord.GuildID, channelID) {
		log.Printf("[ready] no permission to post login notice in %s", channelID)
		return
	}
	if _, err := b.session.ChannelMessageSendEmbed(channelID, loginNoticeEmbed(b.cfg.Bot.DisplayName)); err != nil {
		log.Printf("[ready] login notice failed: %v", err)
	}
}

func loginNoticeEmbed(displayName string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%sがログインしました", displayName),
		Color: 0x57F287,
	}
}

// errorReplyEmbed marks a failed reply as an error, distinct from normal chat.
func errorReplyEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "応答の生成に失敗しました",
		Description: "ごめん、今うまく考えられなかった。もう一度言ってみて。",
		Color:       0xED4245,
	}
}

// scope resolves channel/thread and whether the message is in an allowed category.
func (b *Bot) scope(m *discordgo.MessageCreate) (channelID string, threadID *string, ok bool) {
	if m.GuildID == "" || m.GuildID != b.cfg.Discord.GuildID {
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
	for _, c := range b.cfg.Discord.CategoryIDs {
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

// hasReadPermission checks view+history only (no send): for read-only polling.
func (b *Bot) hasReadPermission(guildID, channelID string) bool {
	perms, err := b.session.UserChannelPermissions(b.botID, channelID)
	if err != nil {
		return false
	}
	return perms&discordgo.PermissionViewChannel != 0 &&
		perms&discordgo.PermissionReadMessageHistory != 0
}

// isReadOnly reports whether a channel is read-only for the bot.
// The bot never replies in or posts to these channels.
func (b *Bot) isReadOnly(channelID string) bool {
	for _, id := range b.cfg.Discord.ReadonlyChannelIDs {
		if id == channelID {
			return true
		}
	}
	return false
}

// channelName resolves a channel ID to "#name", cached per process.
func (b *Bot) channelName(channelID string) string {
	b.chNamesMu.Lock()
	defer b.chNamesMu.Unlock()
	if n, ok := b.chNames[channelID]; ok {
		return n
	}
	name := channelID
	if ch, err := b.session.Channel(channelID); err == nil && ch.Name != "" {
		name = "#" + ch.Name
	}
	b.chNames[channelID] = name
	return name
}

// PollReadonlyChannel fetches recent messages from a read-only channel and
// stores them (upsert by Discord message ID, so reruns are idempotent).
func (b *Bot) PollReadonlyChannel(ctx context.Context, channelID string) error {
	if !b.hasReadPermission(b.cfg.Discord.GuildID, channelID) {
		return fmt.Errorf("no read permission for %s", channelID)
	}
	msgs, err := b.session.ChannelMessages(channelID, 20, "", "", "")
	if err != nil {
		return err
	}
	// API returns newest first; store oldest first for chronological IDs.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	for _, dm := range msgs {
		if dm.Author == nil {
			continue
		}
		attachmentsJSON, _ := json.Marshal(dm.Attachments)
		_, err := b.store.SaveMessage(ctx, store.Message{
			GuildID: b.cfg.Discord.GuildID, ChannelID: channelID,
			UserID: dm.Author.ID, Username: dm.Author.Username,
			Content: dm.Content, Attachments: string(attachmentsJSON), DiscordID: dm.ID,
		})
		if err != nil {
			return err
		}
	}
	log.Printf("[readonly] polled %s: %d messages", channelID, len(msgs))
	return nil
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
			b.botName = me.Username
		}
	}
	channelID, threadID, ok := b.scope(m)
	if !ok {
		return
	}
	if b.isReadOnly(m.ChannelID) {
		return // read-only: polling stores it, never reply here
	}
	if !b.hasPermission(m.GuildID, m.ChannelID) {
		return
	}
	mentioned := isMentionOf(b.botID, m.Content, m.Mentions)
	isReply := m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil && m.ReferencedMessage.Author.ID == b.botID
	if !mentioned && !isReply && !b.cfg.Bot.RespondAll {
		return // non-trigger messages are handled by the random scheduler
	}
	b.handleReply(m, channelID, threadID)
}

// scopeOf returns the memory scope: the thread when in one, else the channel.
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

	// Show a working embed only if the reply is slow (fixed 3s threshold).
	// Fast replies never display it; slow phases update it via runTool.
	st := postStatusDelayed(b.session, m.ChannelID,
		statusDelay, "考え中……", "回答を準備しています")

	attachmentsJSON, _ := json.Marshal(m.Attachments)
	stMsg := store.Message{
		GuildID: m.GuildID, ChannelID: channelID, ThreadID: threadID,
		UserID: m.Author.ID, Username: m.Author.Username,
		Content: m.Content, Attachments: string(attachmentsJSON), DiscordID: m.ID,
	}
	msgID, err := b.store.SaveMessage(ctx, stMsg)
	if err != nil {
		log.Printf("[reply] SaveMessage failed: %v", err)
	}

	// Enrich user content with image/link context before LLM call.
	enriched, images := b.enrichContent(ctx, m)
	if len(images) > 0 {
		log.Printf("[reply] attaching %d image(s) as vision input msgID=%d", len(images), msgID)
	}

	mctx := memory.Build(ctx, b.store, b.llm, m.GuildID, channelID, threadID, b.cfg.Memory.HistoryLimit, b.cfg.Memory.TopK, m.Content)

	// Background from read-only channels (periodic polling stores them).
	if ids := b.cfg.Discord.ReadonlyChannelIDs; len(ids) > 0 {
		if ro, err := b.store.RecentFromChannels(ctx, m.GuildID, ids, 10); err != nil {
			log.Printf("[reply] readonly context failed: %v", err)
		} else if len(ro) > 0 {
			mctx.Readonly = ro
			mctx.ReadonlyNames = map[string]string{}
			for _, rm := range ro {
				mctx.ReadonlyNames[rm.ChannelID] = b.channelName(rm.ChannelID)
			}
		}
	}

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
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: m.Author.Username + ": " + enriched, Images: images})

	scope := channelID
	if threadID != nil {
		scope = *threadID
	}
	reply, toolErr := b.chatWithTools(ctx, msgs, msgID, st, toolEnv{
		guildID:   m.GuildID,
		scopeKey:  scope,
		channelID: m.ChannelID,
		username:  m.Author.Username,
	})
	st.Done()
	if toolErr != nil {
		log.Printf("[reply] chat failed guild=%s ch=%s thread=%s msgID=%d hist=%d mem=%d: %v",
			m.GuildID, channelID, threadStr(threadID), msgID, len(mctx.History), len(mctx.Memories), toolErr)
		// Post a distinct error embed instead of a normal-looking reply.
		if _, err := b.session.ChannelMessageSendEmbed(m.ChannelID, errorReplyEmbed()); err != nil {
			log.Printf("[reply] error embed failed ch=%s msgID=%d: %v", m.ChannelID, msgID, err)
		}
		return
	}
	// The "Username: content" history format sometimes leaks into the
	// model's own reply as a leading "bot-name:" prefix. Strip it.
	reply = b.stripSelfPrefix(reply)
	if err := b.sendSplit(m.ChannelID, reply); err != nil {
		log.Printf("[reply] send failed ch=%s msgID=%d: %v", m.ChannelID, msgID, err)
		return
	}
	// Best-effort reaction to the trigger message; never blocks the reply.
	go b.maybeReact(m)

	if _, err := b.store.SaveMessage(ctx, store.Message{
		GuildID: m.GuildID, ChannelID: channelID, ThreadID: threadID,
		UserID: b.botID, Username: b.cfg.Bot.DisplayName, Content: reply,
		Attachments: "[]",
	}); err != nil {
		log.Printf("[reply] SaveMessage(assistant) failed: %v", err)
	}
	// Long-term memory is stored only via the remember tool (LLM judgment).
	// No length-based auto-save: every memorized fact is traceable to a
	// model decision visible in tool_runs.
}

// notifyRemembered tells the channel that a new fact about the user was
// memorized. Best-effort: failures are logged and ignored.
func (b *Bot) notifyRemembered(channelID, username, summary string) {
	_, err := b.session.ChannelMessageSendEmbed(channelID, &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%sさんについて新しいことを覚えました！", username),
		Description: textutil.Truncate(summary, 200),
		Color:       0x57F287,
	})
	if err != nil {
		log.Printf("[reply] memory notice failed: %v", err)
	}
}

// stripSelfPrefix removes a leading "bot-name:" / mention prefix that the
// model sometimes echoes from the "Username: content" history format.
func (b *Bot) stripSelfPrefix(reply string) string {
	names := []string{b.botName, b.cfg.Bot.DisplayName, "dis-llm-bot", "dls-llm-bot"}
	var mentions []string
	if b.botID != "" {
		mentions = []string{"<@" + b.botID + ">", "<@!" + b.botID + ">"}
	}
	t := strings.TrimSpace(reply)
	for changed := true; changed; {
		changed = false
		// Bare leading mention ("<@id> hello"), no colon.
		for _, tok := range mentions {
			if rest, ok := strings.CutPrefix(t, tok); ok {
				t = strings.TrimSpace(rest)
				changed = true
			}
		}
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			for _, sep := range []string{":", "："} {
				if rest, ok := strings.CutPrefix(t, n+sep); ok {
					t = strings.TrimSpace(rest)
					changed = true
				}
			}
		}
	}
	return t
}

// toolEnv carries reply context into tool execution (for remember/save).
type toolEnv struct {
	guildID   string
	scopeKey  string // channelID or threadID: memory scope
	channelID string // where to send notices (the thread/channel of the trigger)
	username  string
}

func (b *Bot) chatWithTools(ctx context.Context, msgs []llm.ChatMessage, msgID int64, st *Status, env toolEnv) (string, error) {
	toolDefs := llm.DefaultTools(b.cfg.Shell.Enabled)
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
				res := b.runTool(ctx, tc, msgID, st, env)
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

func (b *Bot) runTool(ctx context.Context, tc llm.ToolCall, msgID int64, st *Status, env toolEnv) string {
	start := time.Now()
	// Cap each tool call so one hanging site/endpoint can't stall the reply.
	// SaveToolRun below still uses the parent ctx so the record persists.
	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if title, desc := statusTextForTool(tc.Function.Name, tc.Function.Arguments); st != nil {
		st.Update(title, desc)
	}
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
		r, err := tools.BraveSearch(tctx, b.cfg.Brave.APIKey, q, n)
		if err != nil {
			result = "search error: " + err.Error()
		} else {
			result = r
		}
	case "fetch_page":
		u, _ := args["url"].(string)
		r, err := tools.FetchPage(tctx, u)
		if err != nil {
			result = "fetch error: " + err.Error()
		} else {
			result = r
		}
	case "exec_shell":
		if !b.cfg.Shell.Enabled {
			result = "shell is disabled"
			break
		}
		cmdStr, _ := args["command"].(string)
		to := b.cfg.Shell.TimeoutSec
		if f, ok := args["timeout"].(float64); ok && f > 0 {
			to = int(f)
		}
		r, err := tools.ExecShell(tctx, b.cfg.Shell.Image, cmdStr, to)
		if err != nil {
			result = "shell error: " + err.Error()
		} else {
			result = r
		}
	case "remember":
		fact, _ := args["fact"].(string)
		imp := 2
		if f, ok := args["importance"].(float64); ok {
			imp = int(f)
			if imp < 1 {
				imp = 1
			}
			if imp > 5 {
				imp = 5
			}
		}
		if strings.TrimSpace(fact) == "" {
			result = "remember error: empty fact"
			break
		}
		embs, err := b.llm.Embed(tctx, []string{fact})
		if err != nil {
			result = "remember error: " + err.Error()
		} else if len(embs) == 0 {
			result = "remember error: no embedding returned"
		} else if near, derr := b.store.NearestMemory(ctx, env.guildID, embs[0]); derr != nil {
			result = "remember error: " + derr.Error()
		} else if near != nil && store.IsDuplicate(near.Distance) {
			log.Printf("[tool] remember skipped as duplicate of #%d (dist=%.3f)", near.ID, near.Distance)
			result = "already remembered (duplicate skipped, no notification sent)"
		} else if err := b.store.SaveMemory(ctx, env.guildID, env.scopeKey, "fact", fact, embs[0], imp); err != nil {
			result = "remember error: " + err.Error()
		} else {
			b.notifyRemembered(env.channelID, env.username, fact)
			result = "remembered"
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
	for _, p := range []string{"search error:", "fetch error:", "shell error:", "remember error:", "unknown tool:"} {
		if strings.HasPrefix(result, p) {
			return true
		}
	}
	return false
}

// enrichContent notes image attachments (URLs are passed to the LLM as
// vision image_url parts by the caller) and pre-fetches the first URL in
// the message so the LLM has concrete material.
func (b *Bot) enrichContent(ctx context.Context, m *discordgo.MessageCreate) (string, []string) {
	content := m.Content
	var images []string
	for _, a := range m.Attachments {
		if strings.HasPrefix(a.ContentType, "image/") {
			images = append(images, a.URL)
			content += fmt.Sprintf("\n[attached image %s (%s, %d bytes) — also provided as vision input below]", a.Filename, a.ContentType, a.Size)
		}
	}
	// first http URL in content: fetch text eagerly (tool remains for follow-ups).
	// Short budget: a hanging prefetch must not stall the first LLM call.
	// Tweet links go through the fxtwitter API so attached images are visible.
	if u := firstURL(content); u != "" {
		pctx, pcancel := context.WithTimeout(ctx, 12*time.Second)
		if tools.TweetStatusID(u) != "" {
			txt, imgs, err := tools.FetchTweet(pctx, u)
			if err == nil {
				content += "\n\n[linked tweet]\n" + textutil.Truncate(txt, 3000)
				images = append(images, imgs...)
			} else {
				log.Printf("[enrich] tweet prefetch failed %s: %v", u, err)
			}
		} else if txt, err := tools.FetchPage(pctx, u); err == nil {
			content += "\n\n[linked page content]\n" + textutil.Truncate(txt, 3000)
		} else {
			log.Printf("[enrich] link prefetch failed %s: %v", u, err)
		}
		pcancel()
	}
	return content, images
}

func firstURL(s string) string {
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			return strings.Trim(f, "<>.,)\"'")
		}
	}
	return ""
}

func (b *Bot) sendSplit(channelID, text string) error {
	if b.isReadOnly(channelID) {
		return fmt.Errorf("refusing to post in read-only channel %s", channelID)
	}
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
// Read-only channels are excluded: the bot never posts there.
func (b *Bot) ActiveChannels() ([]ChannelSnapshot, error) {
	allowed := map[string]bool{}
	for _, c := range b.cfg.Discord.CategoryIDs {
		allowed[c] = true
	}
	chs, err := b.session.GuildChannels(b.cfg.Discord.GuildID)
	if err != nil {
		return nil, err
	}
	var out []ChannelSnapshot
	for _, ch := range chs {
		if ch.Type == discordgo.ChannelTypeGuildText && allowed[ch.ParentID] && !b.isReadOnly(ch.ID) {
			out = append(out, ChannelSnapshot{ChannelID: ch.ID, GuildID: b.cfg.Discord.GuildID, LastMsgID: ch.LastMessageID})
		}
	}
	return out, nil
}

func (b *Bot) RandomSay(channelID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	guildID := b.cfg.Discord.GuildID
	hist, _ := b.store.RecentByScope(ctx, guildID, channelID, nil, 8)
	// Use the latest user message as the memory query so related long-term
	// memories are recalled, like in replies.
	query := ""
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].UserID != b.botID && strings.TrimSpace(hist[i].Content) != "" {
			query = hist[i].Content
			break
		}
	}
	mctx := memory.Build(ctx, b.store, b.llm, guildID, channelID, nil, 8, 3, query)
	// Surface read-only channel topics so random talk can pick them up.
	if ids := b.cfg.Discord.ReadonlyChannelIDs; len(ids) > 0 {
		if ro, err := b.store.RecentFromChannels(ctx, guildID, ids, 5); err != nil {
			log.Printf("[random] readonly context failed: %v", err)
		} else if len(ro) > 0 {
			mctx.Readonly = ro
			mctx.ReadonlyNames = map[string]string{}
			for _, rm := range ro {
				mctx.ReadonlyNames[rm.ChannelID] = b.channelName(rm.ChannelID)
			}
		}
	}
	var msgs []llm.ChatMessage
	msgs = append(msgs, llm.ChatMessage{Role: "system", Content: b.persona + "\nYou may start a light casual conversation. Keep it short. You may pick up topics from other channels or memories shown above. If nothing interesting comes to mind, reply with exactly: SKIP\n" + memory.FormatPrompt(mctx)})
	for _, h := range mctx.History {
		role := "user"
		if h.UserID == b.botID {
			role = "assistant"
		}
		msgs = append(msgs, llm.ChatMessage{Role: role, Content: h.Username + ": " + h.Content})
	}
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: "何か一言どうぞ（つまらなければSKIPとだけ返して）"})
	out, err := b.llm.Chat(ctx, msgs, nil)
	if err != nil {
		log.Printf("[random] llm failed ch=%s: %v", channelID, err)
		return
	}
	if strings.TrimSpace(out.Content) == "" || strings.TrimSpace(out.Content) == "SKIP" {
		log.Printf("[random] skipped ch=%s (SKIP)", channelID)
		return
	}
	reply := b.stripSelfPrefix(out.Content)
	if !b.hasPermission(b.cfg.Discord.GuildID, channelID) {
		log.Printf("[random] no permission ch=%s", channelID)
		return
	}
	if err := b.sendSplit(channelID, reply); err != nil {
		log.Printf("[random] send failed ch=%s: %v", channelID, err)
		return
	}
	log.Printf("[random] said ch=%s replyLen=%d", channelID, len(reply))
	_, _ = b.store.SaveMessage(ctx, store.Message{
		GuildID: b.cfg.Discord.GuildID, ChannelID: channelID,
		UserID: b.botID, Username: b.cfg.Bot.DisplayName, Content: reply, Attachments: "[]",
	})
}
