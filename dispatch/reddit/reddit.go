package reddit

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"
)

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

func FetchSubreddit(ctx context.Context, sub string, limit int) ([]Entry, error) {
	url := fmt.Sprintf("https://www.reddit.com/r/%s/new.rss?limit=%d", sub, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Reddit wants a descriptive UA; generic ones get throttled harder.
	req.Header.Set("User-Agent", "personal soccer team news bot by placy2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit rss %s: %s", sub, resp.Status)
	}

	var f feed
	if err := xml.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, err
	}
	return f.Entries, nil
}

// OldLink returns the old.reddit.com link for a given href.
func OldLink(href string) string {
	return "https://old.reddit.com" + href
}
