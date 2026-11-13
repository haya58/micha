package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"

	"dis-llm-bot/internal/config"
)

func TestIsMentionOfDirect(t *testing.T) {
	if !isMentionOf("123", "hey <@123> hello", nil) {
		t.Fatal("should detect <@id> mention")
	}
	if !isMentionOf("123", "hey <@!123> hello", nil) {
		t.Fatal("should detect <@!id> mention")
	}
	if isMentionOf("123", "hey <@456> hello", nil) {
		t.Fatal("should not match other user")
	}
}

func TestIsMentionOfViaList(t *testing.T) {
	mentions := []*discordgo.User{{ID: "123"}}
	if !isMentionOf("123", "hey", mentions) {
		t.Fatal("should detect mention via list")
	}
	if isMentionOf("999", "hey", mentions) {
		t.Fatal("should not match missing id")
	}
}

func TestAllowedCategory(t *testing.T) {
	b := &Bot{cfg: &config.Config{CategoryIDs: []string{"cat1", "cat2"}}}
	if !b.allowedCategory("cat1") || !b.allowedCategory("cat2") {
		t.Fatal("allowed categories not matched")
	}
	if b.allowedCategory("other") {
		t.Fatal("unknown category must not match")
	}
}
