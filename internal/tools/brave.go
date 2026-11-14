package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"dis-llm-bot/internal/textutil"
)

// braveSearchBaseURL is a var so tests can point it at a local server.
var braveSearchBaseURL = "https://api.search.brave.com/res/v1/web/search"

// BraveSearch calls Brave Web Search API. Requires BRAVE_API_KEY.
func BraveSearch(ctx context.Context, apiKey, query string, count int) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("BRAVE_API_KEY is not set")
	}
	if count <= 0 || count > 10 {
		count = 5
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", count))
	params.Set("text_decorations", "false")
	req, err := http.NewRequestWithContext(ctx, "GET", braveSearchBaseURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("brave status %d: %s", resp.StatusCode, textutil.Truncate(string(raw), 300))
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
		return textutil.Truncate(string(raw), 2000), nil
	}
	out := ""
	for i, r := range parsed.Web.Results {
		desc := textutil.Truncate(r.Description, 400)
		out += fmt.Sprintf("[%d] %s\n%s\n%s\n\n", i+1, r.Title, r.URL, desc)
	}
	if out == "" {
		out = "no results"
	}
	return textutil.Truncate(out, 4000), nil
}
