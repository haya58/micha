package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dis-llm-bot/internal/textutil"
)

// FetchPage downloads a URL and extracts rough readable text.
// SSRF guard: rejects private/loopback/link-local hosts.
// Tweet links are routed to the fxtwitter API (X pages require login).
func FetchPage(ctx context.Context, rawURL string) (string, error) {
	if TweetStatusID(rawURL) != "" {
		text, imgs, err := FetchTweet(ctx, rawURL)
		if err != nil {
			return "", err
		}
		for _, u := range imgs {
			text += "\n[image] " + u
		}
		return text, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http/https allowed")
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid url")
	}
	// Bound DNS explicitly: the caller's ctx can be minutes long and a
	// hanging resolver would stall the whole reply (observed in prod logs).
	dnsCtx, dnsCancel := context.WithTimeout(ctx, 8*time.Second)
	defer dnsCancel()
	ips, err := net.DefaultResolver.LookupIPAddr(dnsCtx, host)
	if err != nil {
		return "", fmt.Errorf("dns failed: %w", err)
	}
	for _, ip := range ips {
		if !ip.IP.IsGlobalUnicast() || ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast() {
			return "", fmt.Errorf("private IP blocked: %s", host)
		}
	}
	ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx2, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "dis-llm-bot/0.1 (+prototype)")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	text := stripHTML(string(raw))
	if isBlockedPage(text) {
		return "", fmt.Errorf("site blocks automated fetching: %s", rawURL)
	}
	return fmt.Sprintf("URL: %s\n\n%s", rawURL, textutil.Truncate(text, 12000)), nil
}

// botBlockMarkers matches bot-protection interstitials (Vercel/Cloudflare
// checkpoints, captchas) whose extracted text is junk for the LLM.
func isBlockedPage(text string) bool {
	lower := strings.ToLower(text)
	for _, m := range []string{
		"security checkpoint",
		"just a moment",
		"verify you are human",
		"are you a robot",
		"access denied",
	} {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func stripHTML(s string) string {
	// Crude but dependency-free and linear: single pass, lowercase copy made
	// once. (A previous version re-lowercased the tail on every byte: O(n^2)
	// and minutes of CPU on large pages.)
	low := strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if strings.HasPrefix(low[i:], "<script") || strings.HasPrefix(low[i:], "<style") {
			// Skip the whole script/style block.
			tag := "script"
			if strings.HasPrefix(low[i:], "<style") {
				tag = "style"
			}
			end := strings.Index(low[i:], "</"+tag)
			if end < 0 {
				break
			}
			i += end
			continue
		}
		if s[i] == '<' {
			if j := strings.IndexByte(s[i:], '>'); j >= 0 {
				i += j + 1
			} else {
				break
			}
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	parts := strings.Fields(b.String())
	return textutil.Truncate(strings.Join(parts, " "), 15000)
}
