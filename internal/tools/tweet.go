package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"dis-llm-bot/internal/textutil"
)

// fxtwitterBaseURL is a var so tests can point it at a local server.
var fxtwitterBaseURL = "https://api.fxtwitter.com"

var tweetURLRe = regexp.MustCompile(`^(?:https?://)?(?:www\.)?(?:twitter\.com|x\.com)/[A-Za-z0-9_]+/status/(\d+)`)

// TweetStatusID extracts the status ID from a twitter.com/x.com URL.
// Returns "" when the URL is not a tweet link.
func TweetStatusID(rawURL string) string {
	m := tweetURLRe.FindStringSubmatch(rawURL)
	if m == nil {
		return ""
	}
	return m[1]
}

// FetchTweet retrieves tweet text and photo URLs via the fxtwitter API.
// X's own pages require login, so plain fetching cannot see images.
func FetchTweet(ctx context.Context, statusURL string) (string, []string, error) {
	id := TweetStatusID(statusURL)
	if id == "" {
		return "", nil, fmt.Errorf("not a tweet URL: %s", statusURL)
	}
	ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx2, "GET", fxtwitterBaseURL+"/i/status/"+id, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "dis-llm-bot/0.1 (+prototype)")
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("fxtwitter status %d: %s", resp.StatusCode, textutil.Truncate(string(raw), 200))
	}
	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Tweet   *struct {
			Text   string `json:"text"`
			Author *struct {
				Name       string `json:"name"`
				ScreenName string `json:"screen_name"`
			} `json:"author"`
			Media *struct {
				Photos []struct {
					URL string `json:"url"`
				} `json:"photos"`
			} `json:"media"`
		} `json:"tweet"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", nil, fmt.Errorf("fxtwitter decode: %w", err)
	}
	if parsed.Tweet == nil {
		msg := parsed.Message
		if msg == "" {
			msg = "tweet unavailable (deleted or protected?)"
		}
		return "", nil, fmt.Errorf("fxtwitter: %s", msg)
	}
	text := parsed.Tweet.Text
	if parsed.Tweet.Author != nil {
		text = fmt.Sprintf("%s (@%s): %s", parsed.Tweet.Author.Name, parsed.Tweet.Author.ScreenName, text)
	}
	var images []string
	if parsed.Tweet.Media != nil {
		for _, p := range parsed.Tweet.Media.Photos {
			if p.URL != "" {
				images = append(images, p.URL)
			}
		}
	}
	return fmt.Sprintf("URL: %s\n\n%s", statusURL, textutil.Truncate(text, 4000)), images, nil
}
