package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration.
// One process = one persona = one guild + one logical DB (DATABASE_URL points to
// a database dedicated to this bot/persona).
type Config struct {
	BotID             string
	DisplayName       string
	DiscordToken      string
	GuildID           string
	CategoryIDs       []string
	DatabaseURL       string
	OpenAIBaseURL     string
	OpenAIAPIKey      string
	OpenAIModel       string
	EmbeddingModel    string
	EmbeddingDim      int
	BraveAPIKey       string
	PersonaPath       string
	RandomProb        float64
	RandomCooldownSec int
	HistoryLimit      int
	MemoryTopK        int
	ShellEnabled      bool
	ShellImage        string
	ShellTimeoutSec   int
	HealthAddr        string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() (*Config, error) {
	c := &Config{
		BotID:             getenv("BOT_ID", "default-bot"),
		DisplayName:       getenv("BOT_DISPLAY_NAME", "dis-llm-bot"),
		DiscordToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		GuildID:           os.Getenv("DISCORD_GUILD_ID"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		OpenAIBaseURL:     getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:       getenv("OPENAI_MODEL", "gpt-4o-mini"),
		EmbeddingModel:    getenv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		EmbeddingDim:      1536,
		BraveAPIKey:       os.Getenv("BRAVE_API_KEY"),
		PersonaPath:       os.Getenv("PERSONA_PATH"),
		RandomProb:        0.02,
		RandomCooldownSec: 1800,
		HistoryLimit:      20,
		MemoryTopK:        5,
		ShellEnabled:      getenv("SHELL_ENABLED", "true") == "true",
		ShellImage:        getenv("SHELL_IMAGE", "alpine:latest"),
		ShellTimeoutSec:   30,
		HealthAddr:        getenv("HEALTH_ADDR", ":8080"),
	}
	if v := os.Getenv("DISCORD_CATEGORY_IDS"); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				c.CategoryIDs = append(c.CategoryIDs, s)
			}
		}
	}
	if v := os.Getenv("RANDOM_TALK_PROBABILITY"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.RandomProb = f
		}
	}
	if v := os.Getenv("RANDOM_TALK_COOLDOWN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.RandomCooldownSec = n
		}
	}
	if v := os.Getenv("EMBEDDING_DIM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.EmbeddingDim = n
		}
	}
	if v := os.Getenv("SHELL_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.ShellTimeoutSec = n
		}
	}
	if c.DiscordToken == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN is required")
	}
	if c.GuildID == "" {
		return nil, fmt.Errorf("DISCORD_GUILD_ID is required")
	}
	if len(c.CategoryIDs) == 0 {
		return nil, fmt.Errorf("DISCORD_CATEGORY_IDS is required (comma separated)")
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (dedicated DB per bot/persona)")
	}
	if c.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}
	return c, nil
}
