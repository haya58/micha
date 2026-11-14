package scheduler

import (
	"context"
	"log"
	"math/rand"
	"time"

	"dis-llm-bot/internal/config"
	"dis-llm-bot/internal/discord"
)

// RandomTalk occasionally triggers a casual message in active channels.
func RandomTalk(cfg *config.Config, b *discord.Bot, stop <-chan struct{}) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	lastTalk := map[string]time.Time{}
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// Per-tick chance is probability*15 (tick is 5 min).
			if r.Float64() > cfg.RandomTalk.Probability*15 {
				log.Printf("[random] tick skipped by probability")
				continue
			}
			chs, err := b.ActiveChannels()
			if err != nil || len(chs) == 0 {
				continue
			}
			ch := chs[r.Intn(len(chs))]
			if time.Since(lastTalk[ch.ChannelID]) < time.Duration(cfg.RandomTalk.CooldownSec)*time.Second {
				log.Printf("[random] tick skipped by cooldown ch=%s", ch.ChannelID)
				continue
			}
			lastTalk[ch.ChannelID] = time.Now()
			log.Printf("[random] triggered ch=%s", ch.ChannelID)
			b.RandomSay(ch.ChannelID)
		}
	}
}

// PollReadonly periodically stores recent messages from read-only channels
// so their topics surface in replies and random talk. Stops on channel close.
func PollReadonly(cfg *config.Config, b *discord.Bot, stop <-chan struct{}) {
	if len(cfg.Discord.ReadonlyChannelIDs) == 0 {
		return
	}
	interval := time.Duration(cfg.Discord.ReadonlyPollSec) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	poll := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, id := range cfg.Discord.ReadonlyChannelIDs {
			if err := b.PollReadonlyChannel(ctx, id); err != nil {
				log.Printf("[readonly] poll %s failed: %v", id, err)
			}
		}
	}
	poll() // initial fill at startup
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			poll()
		}
	}
}
