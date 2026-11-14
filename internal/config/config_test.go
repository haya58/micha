package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CONFIG_PATH", "/nonexistent-config.toml")
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_GUILD_ID", "guild1")
	t.Setenv("DISCORD_CATEGORY_IDS", "cat1, cat2")
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("OPENAI_API_KEY", "key")
}

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFromTOML(t *testing.T) {
	setRequired(t)
	// Empty env defers to TOML (env wins only when non-empty).
	t.Setenv("DISCORD_CATEGORY_IDS", "")
	t.Setenv("CONFIG_PATH", writeTOML(t, `
bot_id = "file-bot"
display_name = "file-name"
openai_model = "file-model"
shell_enabled = false
discord_category_ids = ["a", "b", "c"]
`))
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
	if len(c.CategoryIDs) != 3 {
		t.Fatalf("got %v", c.CategoryIDs)
	}
}

func TestEnvOverridesTOML(t *testing.T) {
	setRequired(t)
	t.Setenv("CONFIG_PATH", writeTOML(t, "openai_model = \"file-model\"\n"))
	t.Setenv("OPENAI_MODEL", "env-model")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAIModel != "env-model" {
		t.Fatalf("got %q", c.OpenAIModel)
	}
}

func TestLoadBrokenTOML(t *testing.T) {
	setRequired(t)
	t.Setenv("CONFIG_PATH", writeTOML(t, "this is = = broken"))
	if _, err := Load(); err == nil {
		t.Fatal("expected error for broken TOML")
	}
}

func TestLoadMultipleCategories(t *testing.T) {
	setRequired(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.CategoryIDs) != 2 || c.CategoryIDs[0] != "cat1" || c.CategoryIDs[1] != "cat2" {
		t.Fatalf("CategoryIDs = %v", c.CategoryIDs)
	}
	if c.GuildID != "guild1" {
		t.Fatalf("GuildID = %q", c.GuildID)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	setRequired(t)
	t.Setenv("DISCORD_BOT_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DISCORD_BOT_TOKEN")
	}
}

func TestLoadMissingCategories(t *testing.T) {
	setRequired(t)
	t.Setenv("DISCORD_CATEGORY_IDS", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DISCORD_CATEGORY_IDS")
	}
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAIModel == "" || c.EmbeddingDim <= 0 {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if !c.ShellEnabled {
		t.Fatal("SHELL_ENABLED should default to true")
	}
}

func TestParseExtraHeaders(t *testing.T) {
	got := ParseExtraHeaders("x-opencode-session: ses_123, X-Custom: abc")
	if got["x-opencode-session"] != "ses_123" || got["X-Custom"] != "abc" {
		t.Fatalf("got %v", got)
	}
	if len(ParseExtraHeaders("")) != 0 {
		t.Fatal("empty input should yield empty map")
	}
	// malformed entries are skipped, valid ones kept
	got = ParseExtraHeaders("bogus, ok: yes, empty:")
	if len(got) != 1 || got["ok"] != "yes" {
		t.Fatalf("got %v", got)
	}
}

func TestLoadExtraHeaders(t *testing.T) {
	setRequired(t)
	t.Setenv("OPENAI_EXTRA_HEADERS", "x-opencode-session: ses_123")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenAIExtraHeaders["x-opencode-session"] != "ses_123" {
		t.Fatalf("got %v", c.OpenAIExtraHeaders)
	}
}

func TestRespondAllDefaultOff(t *testing.T) {
	setRequired(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.RespondAll {
		t.Fatal("RESPOND_ALL should default to false")
	}
}

func TestRespondAllOn(t *testing.T) {
	setRequired(t)
	t.Setenv("RESPOND_ALL", "true")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.RespondAll {
		t.Fatal("RESPOND_ALL=true should enable mention-free mode")
	}
}

func TestReadonlyChannels(t *testing.T) {
	setRequired(t)
	if c, err := Load(); err != nil || len(c.ReadonlyChannelIDs) != 0 {
		t.Fatalf("default should be empty: %+v %v", c, err)
	}
	setRequired(t)
	t.Setenv("READONLY_CHANNEL_IDS", "ch1, ch2")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.ReadonlyChannelIDs) != 2 || c.ReadonlyChannelIDs[0] != "ch1" {
		t.Fatalf("got %v", c.ReadonlyChannelIDs)
	}
	if c.ReadonlyPollSec != 300 {
		t.Fatalf("default poll interval should be 300, got %d", c.ReadonlyPollSec)
	}
}
