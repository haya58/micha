package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all runtime configuration.
// One process = one persona = one guild + one logical DB (DATABASE_URL points to
// a database dedicated to this bot/persona).
//
// The sole source is the TOML file (CONFIG_PATH, default "config.toml"),
// organized in sections. Environment variables are not read, except
// CONFIG_PATH itself which locates the file.
type Config struct {
	Bot struct {
		ID          string `toml:"id"`
		DisplayName string `toml:"display_name"`
		RespondAll  bool   `toml:"respond_all"`
	} `toml:"bot"`
	Discord struct {
		Token                string   `toml:"token"`
		GuildID              string   `toml:"guild_id"`
		CategoryIDs          []string `toml:"category_ids"`
		ReadonlyChannelIDs   []string `toml:"readonly_channel_ids"`
		ReadonlyPollSec      int      `toml:"readonly_poll_sec"`
		LoginNoticeChannelID string   `toml:"login_notice_channel_id"`
	} `toml:"discord"`
	Database struct {
		URL string `toml:"url"`
	} `toml:"database"`
	Endpoints map[string]Endpoint `toml:"endpoints"`
	Models    struct {
		// "<endpoint>/<model>", e.g. "opencode/kimi-k3".
		Chat         string `toml:"chat"`
		Embedding    string `toml:"embedding"`
		EmbeddingDim int    `toml:"embedding_dim"`
	} `toml:"models"`
	Brave struct {
		APIKey string `toml:"api_key"`
	} `toml:"brave"`
	Persona struct {
		Path string `toml:"path"`
	} `toml:"persona"`
	RandomTalk struct {
		Probability float64 `toml:"probability"`
		CooldownSec int     `toml:"cooldown_sec"`
	} `toml:"random_talk"`
	Memory struct {
		HistoryLimit       int `toml:"history_limit"`
		TopK               int `toml:"top_k"`
		ReadonlyPerChannel int `toml:"readonly_per_channel"`
	} `toml:"memory"`
	Reaction struct {
		Probability float64 `toml:"probability"`
		CooldownSec int     `toml:"cooldown_sec"`
	} `toml:"reaction"`
	Shell struct {
		Enabled    bool   `toml:"enabled"`
		Image      string `toml:"image"`
		TimeoutSec int    `toml:"timeout_sec"`
	} `toml:"shell"`
	Server struct {
		HealthAddr string `toml:"health_addr"`
	} `toml:"server"`
}

// Endpoint is one named OpenAI-compatible API deployment.
type Endpoint struct {
	BaseURL      string            `toml:"base_url"`
	APIKey       string            `toml:"api_key"`
	ExtraHeaders map[string]string `toml:"extra_headers"`
}

func defaults() *Config {
	c := &Config{}
	c.Bot.ID = "micha-bot"
	c.Bot.DisplayName = "micha-bot"
	c.RandomTalk.Probability = 0.02
	c.RandomTalk.CooldownSec = 1800
	c.Memory.HistoryLimit = 30
	c.Memory.TopK = 8
	c.Memory.ReadonlyPerChannel = 10
	c.Reaction.Probability = 0.15
	c.Reaction.CooldownSec = 600
	c.Discord.ReadonlyPollSec = 300
	c.Shell.Enabled = true
	c.Shell.Image = "alpine:latest"
	c.Shell.TimeoutSec = 30
	c.Server.HealthAddr = ":8080"
	c.Models.EmbeddingDim = 1536
	return c
}

// Load reads built-in defaults, then overlays the TOML file.
// Only keys present in the file override defaults.
// A missing file is an error: create one from config.example.toml.
func Load() (*Config, error) {
	c := defaults()
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config.toml"
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s (copy config.example.toml)", path)
		}
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	if _, err := toml.DecodeFile(path, c); err != nil {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	if c.Discord.Token == "" {
		return nil, fmt.Errorf("discord.token is required")
	}
	if c.Discord.GuildID == "" {
		return nil, fmt.Errorf("discord.guild_id is required")
	}
	if len(c.Discord.CategoryIDs) == 0 {
		return nil, fmt.Errorf("discord.category_ids is required")
	}
	if c.Database.URL == "" {
		return nil, fmt.Errorf("database.url is required (dedicated DB per bot/persona)")
	}
	if _, _, err := c.ResolveModel(c.Models.Chat); err != nil {
		return nil, fmt.Errorf("models.chat: %w", err)
	}
	if _, _, err := c.ResolveModel(c.Models.Embedding); err != nil {
		return nil, fmt.Errorf("models.embedding: %w", err)
	}
	if c.Models.EmbeddingDim <= 0 {
		return nil, fmt.Errorf("models.embedding_dim must be positive (match the embedding model)")
	}
	return c, nil
}

// ResolveModel splits an "<endpoint>/<model>" reference.
func (c *Config) ResolveModel(ref string) (Endpoint, string, error) {
	i := strings.Index(ref, "/")
	if i <= 0 || i == len(ref)-1 {
		return Endpoint{}, "", fmt.Errorf("want <endpoint>/<model>, got %q", ref)
	}
	ep, ok := c.Endpoints[ref[:i]]
	if !ok {
		return Endpoint{}, "", fmt.Errorf("unknown endpoint %q", ref[:i])
	}
	return ep, ref[i+1:], nil
}
