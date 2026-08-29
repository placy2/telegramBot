package reddit

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// redditUserAgent follows Reddit's documented format for identifying a
// client: <platform>:<app ID>:<version> (by /u/<username>). Non-conforming
// UAs are throttled harder.
const redditUserAgent = "go:telegramBot:2.0 (by /u/placy2)"

// client has an explicit timeout, unlike http.DefaultClient (zero value —
// no timeout at all). Paired with a cancellable context, a hung Reddit
// connection can no longer block the bot's update loop forever.
var client = &http.Client{Timeout: 15 * time.Second}

type Entry struct {
	ID        string    `xml:"id"`
	Title     string    `xml:"title"`
	Published time.Time `xml:"published"` // encoding/xml parses RFC3339 into time
	Link      struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
}

type feed struct {
	Entries []Entry `xml:"entry"`
}

// RateLimitError reports a 429 from Reddit's RSS endpoint, including how
// long the caller should wait before retrying the same subreddit — see
// x-ratelimit-reset in the response headers.
type RateLimitError struct {
	Sub        string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("reddit rss %s: rate limited, retry after %s", e.Sub, e.RetryAfter)
}

func FetchSubreddit(ctx context.Context, sub string, limit int) ([]Entry, error) {
	reqURL := fmt.Sprintf("https://www.reddit.com/r/%s/new.rss?limit=%d", url.PathEscape(sub), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", redditUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain (bounded) rather than discard-and-close immediately, so the
		// underlying connection can be reused for the next request.
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, &RateLimitError{Sub: sub, RetryAfter: retryAfter(resp)}
		}
		return nil, fmt.Errorf("reddit rss %s: %s", sub, resp.Status)
	}

	var f feed
	if err := xml.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, err
	}
	return f.Entries, nil
}

// retryAfter reads how long to wait before retrying, preferring Reddit's
// x-ratelimit-reset (seconds until the current window ends) and falling
// back to the standard Retry-After header, then a 60s default matching the
// measured anonymous budget (1 request/minute/IP).
func retryAfter(resp *http.Response) time.Duration {
	if v := resp.Header.Get("x-ratelimit-reset"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return 60 * time.Second
}

// OldLink rewrites a reddit.com URL to the ad-free old.reddit.com host.
// Returns the input unchanged if it isn't a URL we recognize. RSS <link
// href> values are already full URLs (not paths), so this rewrites the
// host rather than concatenating one.
func OldLink(href string) string {
	u, err := url.Parse(href)
	if err != nil || !strings.HasSuffix(u.Host, "reddit.com") {
		return href
	}
	u.Host = "old.reddit.com"
	return u.String()
}
