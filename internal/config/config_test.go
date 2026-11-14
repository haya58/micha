package config

import (
	"os"
	"path/filepath"
	"testing"
)

const requiredTOML = `
[discord]
token = "tok"
guild_id = "guild1"
category_ids = ["cat1", "cat2"]

[database]
url = "postgres://localhost/db"

[openai]
api_key = "key"
`

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)
	return path
}

func TestLoadFromTOML(t *testing.T) {
	writeTOML(t, `
[bot]
id = "file-bot"
display_name = "file-name"

[discord]
token = "tok"
guild_id = "guild1"
category_ids = ["a", "b", "c"]

[database]
url = "postgres://localhost/db"

[openai]
api_key = "key"
model = "file-model"

[shell]
enabled = false
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Bot.ID != "file-bot" || c.Bot.DisplayName != "file-name" {
		t.Fatalf("got %+v", c)
	}
	if c.OpenAI.Model != "file-model" || c.Shell.Enabled {
		t.Fatalf("got %+v", c)
	}
	if len(c.Discord.CategoryIDs) != 3 || c.Discord.CategoryIDs[0] != "a" {
		t.Fatalf("got %v", c.Discord.CategoryIDs)
	}
	if c.Discord.GuildID != "guild1" {
		t.Fatalf("GuildID = %q", c.Discord.GuildID)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	writeTOML(t, `
[discord]
guild_id = "guild1"
category_ids = ["cat1"]

[database]
url = "postgres://localhost/db"

[openai]
api_key = "key"
`)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing discord token")
	}
}

func TestLoadDefaults(t *testing.T) {
	writeTOML(t, requiredTOML)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAI.Model != "gpt-4o-mini" || c.OpenAI.EmbeddingDim != 1536 {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if !c.Shell.Enabled {
		t.Fatal("shell.enabled should default to true")
	}
	if c.Bot.RespondAll {
		t.Fatal("bot.respond_all should default to false")
	}
	if c.Discord.ReadonlyPollSec != 300 {
		t.Fatalf("readonly_poll_sec should default to 300, got %d", c.Discord.ReadonlyPollSec)
	}
}

func TestLoadExtraHeaders(t *testing.T) {
	writeTOML(t, requiredTOML+`
[openai.extra_headers]
x-opencode-session = "ses_123"
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAI.ExtraHeaders["x-opencode-session"] != "ses_123" {
		t.Fatalf("got %v", c.OpenAI.ExtraHeaders)
	}
}

func TestLoadRespondAll(t *testing.T) {
	writeTOML(t, requiredTOML+`
[bot]
respond_all = true
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Bot.RespondAll {
		t.Fatal("respond_all=true should enable mention-free mode")
	}
}

func TestLoadReadonlyChannels(t *testing.T) {
	writeTOML(t, `
[discord]
token = "tok"
guild_id = "guild1"
category_ids = ["cat1"]
readonly_channel_ids = ["ch1", "ch2"]
readonly_poll_sec = 120

[database]
url = "postgres://localhost/db"

[openai]
api_key = "key"
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Discord.ReadonlyChannelIDs) != 2 || c.Discord.ReadonlyChannelIDs[0] != "ch1" {
		t.Fatalf("got %v", c.Discord.ReadonlyChannelIDs)
	}
	if c.Discord.ReadonlyPollSec != 120 {
		t.Fatalf("got %d", c.Discord.ReadonlyPollSec)
	}
}

func TestLoadBrokenTOML(t *testing.T) {
	writeTOML(t, "this is = = broken")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for broken TOML")
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.toml"))
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing config file")
	}
}
