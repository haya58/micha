package textutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateMultibyte(t *testing.T) {
	in := "あいうえお"
	if got := Truncate(in, 3); got != "あいう" {
		t.Fatalf("got %q", got)
	}
	if got := Truncate(in, 10); got != in {
		t.Fatalf("short string must pass through, got %q", got)
	}
	if got := Truncate(in, 0); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestCleanDropsInvalid(t *testing.T) {
	// "あ" is 3 bytes; cutting after 2 yields invalid UTF-8.
	bad := "あ"[:2] + "い"
	if utf8.ValidString(bad) {
		t.Fatal("test setup broken: expected invalid UTF-8")
	}
	cleaned := Clean(bad)
	if !utf8.ValidString(cleaned) {
		t.Fatalf("invalid UTF-8 remains: %q", cleaned)
	}
	if cleaned != "い" {
		t.Fatalf("got %q, want %q", cleaned, "い")
	}
}
