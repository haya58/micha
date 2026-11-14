// Package textutil holds small UTF-8-safe string helpers shared by the bot.
package textutil

import "strings"

// Truncate cuts s to at most max runes (byte-slicing would split
// multibyte characters and produce invalid UTF-8 for Postgres).
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// Clean drops invalid UTF-8 byte sequences. Last line of defence before
// persisting LLM/tool output to Postgres (which rejects invalid bytes).
func Clean(s string) string {
	return strings.ToValidUTF8(s, "")
}
