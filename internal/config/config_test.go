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

[endpoints.openai]
base_url = "https://api.openai.com/v1"
api_key = "key"

[models]
chat = "openai/gpt-4o-mini"
embedding = "openai/text-embedding-3-small"
embedding_dim = 1536
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

[endpoints.openai]
base_url = "https://example.com/v1"
api_key = "key"

[models]
chat = "openai/file-model"
embedding = "openai/file-embed"
embedding_dim = 768
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Bot.ID != "file-bot" || c.Bot.DisplayName != "file-name" {
		t.Fatalf("got %+v", c.Bot)
	}
	if c.Discord.GuildID != "guild1" {
		t.Fatalf("GuildID = %q", c.Discord.GuildID)
	}
	if len(c.Discord.CategoryIDs) != 3 || c.Discord.CategoryIDs[0] != "a" {
		t.Fatalf("got %v", c.Discord.CategoryIDs)
	}
	ep, model, err := c.ResolveModel(c.Models.Chat)
	if err != nil {
		t.Fatalf("resolve chat: %v", err)
	}
	if ep.BaseURL != "https://example.com/v1" || model != "file-model" {
		t.Fatalf("got %+v %q", ep, model)
	}
	if _, _, err := c.ResolveModel(c.Models.Embedding); err != nil {
		t.Fatalf("resolve embedding: %v", err)
	}
	if c.Models.EmbeddingDim != 768 {
		t.Fatalf("got %d", c.Models.EmbeddingDim)
	}
	if !c.Shell.Enabled {
		t.Fatal("shell.enabled should default to true (section absent)")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	writeTOML(t, `
[discord]
guild_id = "guild1"
category_ids = ["cat1"]

[database]
url = "postgres://localhost/db"

[endpoints.openai]
base_url = "https://example.com/v1"

[models]
chat = "openai/m"
embedding = "openai/e"
embedding_dim = 1536
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
	if c.Bot.ID != "micha-bot" || c.Bot.DisplayName != "micha-bot" {
		t.Fatalf("defaults not applied: %+v", c.Bot)
	}
	if !c.Shell.Enabled {
		t.Fatal("shell.enabled should default to true")
	}
	if c.Discord.ReadonlyPollSec != 300 {
		t.Fatalf("readonly_poll_sec should default to 300, got %d", c.Discord.ReadonlyPollSec)
	}
	if c.RandomTalk.Probability != 0.02 || c.Memory.TopK != 8 {
		t.Fatalf("got %+v %+v", c.RandomTalk, c.Memory)
	}
	if c.Memory.ReadonlyPerChannel != 10 {
		t.Fatalf("readonly_per_channel should default to 10, got %d", c.Memory.ReadonlyPerChannel)
	}
}

func TestLoadExtraHeaders(t *testing.T) {
	writeTOML(t, requiredTOML+`
[endpoints.openai.extra_headers]
x-opencode-session = "ses_123"
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ep, _, err := c.ResolveModel(c.Models.Chat)
	if err != nil {
		t.Fatal(err)
	}
	if ep.ExtraHeaders["x-opencode-session"] != "ses_123" {
		t.Fatalf("got %v", ep.ExtraHeaders)
	}
}

func TestResolveModelErrors(t *testing.T) {
	writeTOML(t, requiredTOML)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"", "noslash", "/model", "endpoint/", "missing/m"} {
		if _, _, err := c.ResolveModel(ref); err == nil {
			t.Fatalf("expected error for %q", ref)
		}
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
