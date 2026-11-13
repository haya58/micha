package config

import (
	"testing"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_GUILD_ID", "guild1")
	t.Setenv("DISCORD_CATEGORY_IDS", "cat1, cat2")
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("OPENAI_API_KEY", "key")
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
