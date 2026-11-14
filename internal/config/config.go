package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all runtime configuration.
// One process = one persona = one guild + one logical DB (DATABASE_URL points to
// a database dedicated to this bot/persona).
//
// Sources, in ascending precedence: built-in defaults < TOML file
// (CONFIG_PATH, default "config.toml") < environment variables.
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

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

// Load reads built-in defaults, then the TOML file, then environment
// variables. A missing TOML file is silently skipped so env-only
// operation (docker-compose/k8s) keeps working.
func Load() (*Config, error) {
	c := defaults()
	path := getenv("CONFIG_PATH", "config.toml")
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, c); err != nil {
			return nil, fmt.Errorf("config file %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	c.applyEnv()
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

// applyEnv overlays non-empty environment variables onto the config.
func (c *Config) applyEnv() {
	if v := os.Getenv("BOT_ID"); v != "" {
		c.BotID = v
	}
	if v := os.Getenv("BOT_DISPLAY_NAME"); v != "" {
		c.DisplayName = v
	}
	if v := os.Getenv("DISCORD_BOT_TOKEN"); v != "" {
		c.DiscordToken = v
	}
	if v := os.Getenv("DISCORD_GUILD_ID"); v != "" {
		c.GuildID = v
	}
	if v := os.Getenv("DISCORD_CATEGORY_IDS"); v != "" {
		c.CategoryIDs = splitList(v)
	}
	if v := os.Getenv("READONLY_CHANNEL_IDS"); v != "" {
		c.ReadonlyChannelIDs = splitList(v)
	}
	if v := os.Getenv("READONLY_POLL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 60 {
			c.ReadonlyPollSec = n
		}
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.DatabaseURL = v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		c.OpenAIBaseURL = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		c.OpenAIAPIKey = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		c.OpenAIModel = v
	}
	if v := os.Getenv("OPENAI_EMBEDDING_MODEL"); v != "" {
		c.EmbeddingModel = v
	}
	if v := os.Getenv("OPENAI_EXTRA_HEADERS"); v != "" {
		c.OpenAIExtraHeaders = ParseExtraHeaders(v)
	}
	if v := os.Getenv("EMBEDDING_DIM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.EmbeddingDim = n
		}
	}
	if v := os.Getenv("BRAVE_API_KEY"); v != "" {
		c.BraveAPIKey = v
	}
	if v := os.Getenv("PERSONA_PATH"); v != "" {
		c.PersonaPath = v
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
	if v := os.Getenv("RESPOND_ALL"); v != "" {
		c.RespondAll = v == "true"
	}
	if v := os.Getenv("LOGIN_NOTICE_CHANNEL_ID"); v != "" {
		c.LoginNoticeChannelID = v
	}
	if v := os.Getenv("SHELL_ENABLED"); v != "" {
		c.ShellEnabled = v == "true"
	}
	if v := os.Getenv("SHELL_IMAGE"); v != "" {
		c.ShellImage = v
	}
	if v := os.Getenv("SHELL_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.ShellTimeoutSec = n
		}
	}
	if v := os.Getenv("HEALTH_ADDR"); v != "" {
		c.HealthAddr = v
	}
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ParseExtraHeaders parses "Name: value, Name2: value2" into a header map.
// Used for gateways that need extra headers (e.g. x-opencode-session).
// Malformed entries are skipped.
func ParseExtraHeaders(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || name == "" || value == "" {
			continue
		}
		out[name] = value
	}
	return out
}
