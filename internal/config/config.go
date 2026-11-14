package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds all runtime configuration.
// One process = one persona = one guild + one logical DB (DATABASE_URL points to
// a database dedicated to this bot/persona).
//
// The sole source is the TOML file (CONFIG_PATH, default "config.toml").
// Environment variables are intentionally not supported, except CONFIG_PATH
// itself which locates the file.
type Config struct {
	BotID                string            `toml:"bot_id"`
	DisplayName          string            `toml:"display_name"`
	DiscordToken         string            `toml:"discord_bot_token"`
	GuildID              string            `toml:"discord_guild_id"`
	CategoryIDs          []string          `toml:"discord_category_ids"`
	DatabaseURL          string            `toml:"database_url"`
	OpenAIBaseURL        string            `toml:"openai_base_url"`
	OpenAIAPIKey         string            `toml:"openai_api_key"`
	OpenAIModel          string            `toml:"openai_model"`
	OpenAIExtraHeaders   map[string]string `toml:"openai_extra_headers"`
	EmbeddingModel       string            `toml:"openai_embedding_model"`
	EmbeddingDim         int               `toml:"embedding_dim"`
	BraveAPIKey          string            `toml:"brave_api_key"`
	PersonaPath          string            `toml:"persona_path"`
	RandomProb           float64           `toml:"random_talk_probability"`
	RandomCooldownSec    int               `toml:"random_talk_cooldown"`
	RespondAll           bool              `toml:"respond_all"`
	LoginNoticeChannelID string            `toml:"login_notice_channel_id"`
	ReadonlyChannelIDs   []string          `toml:"readonly_channel_ids"`
	ReadonlyPollSec      int               `toml:"readonly_poll_sec"`
	HistoryLimit         int               `toml:"history_limit"`
	MemoryTopK           int               `toml:"memory_top_k"`
	ShellEnabled         bool              `toml:"shell_enabled"`
	ShellImage           string            `toml:"shell_image"`
	ShellTimeoutSec      int               `toml:"shell_timeout"`
	HealthAddr           string            `toml:"health_addr"`
}

func defaults() *Config {
	return &Config{
		BotID:             "default-bot",
		DisplayName:       "dis-llm-bot",
		OpenAIBaseURL:     "https://api.openai.com/v1",
		OpenAIModel:       "gpt-4o-mini",
		EmbeddingModel:    "text-embedding-3-small",
		EmbeddingDim:      1536,
		RandomProb:        0.02,
		RandomCooldownSec: 1800,
		HistoryLimit:      20,
		MemoryTopK:        5,
		ReadonlyPollSec:   300,
		ShellEnabled:      true,
		ShellImage:        "alpine:latest",
		ShellTimeoutSec:   30,
		HealthAddr:        ":8080",
	}
}

// Load reads built-in defaults, then overlays the TOML file.
// Only keys present in the file override defaults.
func Load() (*Config, error) {
	c := defaults()
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config.toml"
	}
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, c); err != nil {
			return nil, fmt.Errorf("config file %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	if c.DiscordToken == "" {
		return nil, fmt.Errorf("discord_bot_token is required")
	}
	if c.GuildID == "" {
		return nil, fmt.Errorf("discord_guild_id is required")
	}
	if len(c.CategoryIDs) == 0 {
		return nil, fmt.Errorf("discord_category_ids is required")
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("database_url is required (dedicated DB per bot/persona)")
	}
	if c.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("openai_api_key is required")
	}
	return c, nil
}
