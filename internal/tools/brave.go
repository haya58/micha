package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BraveSearch calls Brave Web Search API. Requires BRAVE_API_KEY.
func BraveSearch(ctx context.Context, apiKey, query string, count int) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("BRAVE_API_KEY is not set")
	}
	if count <= 0 || count > 10 {
		count = 5
	}
	req, _ := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d&text_decorations=false", urlQuery(query), count), nil)
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")
	httpc := &http.Client{Timeout: 20 * time.Second}
	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("brave status %d: %.300s", resp.StatusCode, raw)
	}
	var parsed struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return string(raw)[:min(2000, len(raw))], nil
	}
	out := ""
	for i, r := range parsed.Web.Results {
		desc := r.Description
		if len(desc) > 400 {
			desc = desc[:400]
		}
		out += fmt.Sprintf("[%d] %s\n%s\n%s\n\n", i+1, r.Title, r.URL, desc)
	}
	if out == "" {
		out = "no results"
	}
	if len(out) > 4000 {
		out = out[:4000]
	}
	return out, nil
}

func urlQuery(s string) string {
	// minimal escaping without net/url import cycle concerns
	out := ""
	for _, r := range s {
		switch {
		case r == ' ':
			out += "%20"
		case r == '+' || r == '&' || r == '?' || r == '#':
			out += fmt.Sprintf("%%%02X", r)
		default:
			out += string(r)
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
