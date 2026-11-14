package config

import (
	"os"
	"path/filepath"
	"testing"
)

const requiredTOML = `
discord_bot_token = "tok"
discord_guild_id = "guild1"
database_url = "postgres://localhost/db"
openai_api_key = "key"
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
	writeTOML(t, requiredTOML+`
bot_id = "file-bot"
display_name = "file-name"
openai_model = "file-model"
shell_enabled = false
discord_category_ids = ["a", "b", "c"]
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.BotID != "file-bot" || c.DisplayName != "file-name" {
		t.Fatalf("got %+v", c)
	}
	if c.OpenAIModel != "file-model" || c.ShellEnabled {
		t.Fatalf("got %+v", c)
	}
	if len(c.CategoryIDs) != 3 || c.CategoryIDs[0] != "a" {
		t.Fatalf("got %v", c.CategoryIDs)
	}
	if c.GuildID != "guild1" {
		t.Fatalf("GuildID = %q", c.GuildID)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	writeTOML(t, `
discord_guild_id = "guild1"
discord_category_ids = ["cat1"]
database_url = "postgres://localhost/db"
openai_api_key = "key"
`)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing discord_bot_token")
	}
}

func TestLoadMissingCategories(t *testing.T) {
	writeTOML(t, `
discord_bot_token = "tok"
discord_guild_id = "guild1"
database_url = "postgres://localhost/db"
openai_api_key = "key"
`)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing discord_category_ids")
	}
}

func TestLoadDefaults(t *testing.T) {
	writeTOML(t, requiredTOML+"discord_category_ids = [\"cat1\", \"cat2\"]\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAIModel != "gpt-4o-mini" || c.EmbeddingDim != 1536 {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if !c.ShellEnabled {
		t.Fatal("shell_enabled should default to true")
	}
	if c.RespondAll {
		t.Fatal("respond_all should default to false")
	}
	if c.ReadonlyPollSec != 300 {
		t.Fatalf("readonly_poll_sec should default to 300, got %d", c.ReadonlyPollSec)
	}
	if len(c.ReadonlyChannelIDs) != 0 {
		t.Fatalf("readonly_channel_ids should default to empty, got %v", c.ReadonlyChannelIDs)
	}
}

func TestLoadExtraHeaders(t *testing.T) {
	writeTOML(t, requiredTOML+"discord_category_ids = [\"cat1\", \"cat2\"]\n[openai_extra_headers]\nx-opencode-session = \"ses_123\"\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAIExtraHeaders["x-opencode-session"] != "ses_123" {
		t.Fatalf("got %v", c.OpenAIExtraHeaders)
	}
}

func TestLoadRespondAll(t *testing.T) {
	writeTOML(t, requiredTOML+"discord_category_ids = [\"cat1\", \"cat2\"]\nrespond_all = true\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.RespondAll {
		t.Fatal("respond_all=true should enable mention-free mode")
	}
}

func TestLoadReadonlyChannels(t *testing.T) {
	writeTOML(t, requiredTOML+"discord_category_ids = [\"cat1\", \"cat2\"]\nreadonly_channel_ids = [\"ch1\", \"ch2\"]\nreadonly_poll_sec = 120\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.ReadonlyChannelIDs) != 2 || c.ReadonlyChannelIDs[0] != "ch1" {
		t.Fatalf("got %v", c.ReadonlyChannelIDs)
	}
	if c.ReadonlyPollSec != 120 {
		t.Fatalf("got %d", c.ReadonlyPollSec)
	}
}

func TestLoadBrokenTOML(t *testing.T) {
	writeTOML(t, "this is = = broken")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for broken TOML")
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.toml"))
	_, err := Load()
	if err == nil {
		t.Fatal("expected error: required fields missing without file or env")
	}
}
