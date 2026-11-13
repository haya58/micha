package scheduler

import (
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
			if r.Float64() > cfg.RandomProb*15 { // 5-min tick scaled
				continue
			}
			chs, err := b.ActiveChannels()
			if err != nil || len(chs) == 0 {
				continue
			}
			ch := chs[r.Intn(len(chs))]
			if time.Since(lastTalk[ch.ChannelID]) < time.Duration(cfg.RandomCooldownSec)*time.Second {
				continue
			}
			lastTalk[ch.ChannelID] = time.Now()
			b.RandomSay(ch.ChannelID)
		}
	}
}
