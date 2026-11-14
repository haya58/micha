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
func FetchPage(ctx context.Context, rawURL string) (string, error) {
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
	// crude but dependency-free: drop tags, collapse whitespace
	var b strings.Builder
	inTag, inScript := false, false
	lower := strings.ToLower(s)
	_ = lower
	for i := 0; i < len(s); i++ {
		if strings.HasPrefix(strings.ToLower(s[i:]), "<script") {
			inScript = true
		}
		if inScript {
			if strings.HasPrefix(strings.ToLower(s[i:]), "</script") {
				inScript = false
			}
			continue
		}
		if s[i] == '<' {
			inTag = true
			continue
		}
		if s[i] == '>' {
			inTag = false
			b.WriteByte(' ')
			continue
		}
		if !inTag {
			b.WriteByte(s[i])
		}
	}
	parts := strings.Fields(b.String())
	out := strings.Join(parts, " ")
	return textutil.Truncate(out, 15000)
}
