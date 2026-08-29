// Package poller owns all outbound Reddit traffic. Reddit's anonymous RSS
// budget is one request per 60s per IP (measured directly against
// r/GlobalOffensive/new.rss on 2026-08-29 — see
// dispatch/docs/modernization-runbook.md), which is far too slow to serve
// inside a single /hype command across several subreddits. Instead, Run
// ticks in the background, fetching one subreddit per tick and persisting
// keyword matches as store.HypeClip rows; /hype then just reads the store.
package poller

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/placy2/telegramBot/dispatch/config"
	"github.com/placy2/telegramBot/dispatch/reddit"
	"github.com/placy2/telegramBot/dispatch/store"
)

const defaultInterval = 75 * time.Second

// Status is a snapshot of the poller's most recent fetch, used by
// SendHypePlays to tell a genuinely quiet feed apart from a broken one.
type Status struct {
	LastSuccess time.Time
	LastSub     string
	LastErr     error
}

var (
	mu     sync.Mutex
	status Status
)

// Snapshot returns the poller's current health. Safe to call concurrently.
func Snapshot() Status {
	mu.Lock()
	defer mu.Unlock()
	return status
}

func setStatus(sub string, err error) {
	mu.Lock()
	defer mu.Unlock()
	status.LastSub = sub
	status.LastErr = err
	if err == nil {
		status.LastSuccess = time.Now()
	}
}

// Run round-robins one subreddit fetch per tick and persists keyword
// matches to store. It blocks until ctx is cancelled, so callers should
// invoke it with `go poller.Run(ctx, cfg)`.
func Run(ctx context.Context, cfg *config.Config) {
	if len(cfg.Subreddits) == 0 {
		log.Println("poller: no subreddits configured, not starting")
		return
	}

	interval := time.Duration(cfg.PollSeconds) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}

	idx := 0
	wait := time.Duration(0) // fetch immediately on startup

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		sub := cfg.Subreddits[idx]
		entries, err := reddit.FetchSubreddit(ctx, sub, cfg.NumPosts)
		setStatus(sub, err)

		if err != nil {
			var rle *reddit.RateLimitError
			if errors.As(err, &rle) {
				// Don't advance idx — retry the same subreddit once the
				// window resets, rather than silently skipping it.
				log.Printf("poller: rate limited on r/%s, retrying in %s", sub, rle.RetryAfter)
				wait = rle.RetryAfter
				continue
			}
			log.Printf("poller: fetch error for r/%s: %v", sub, err)
			idx = (idx + 1) % len(cfg.Subreddits)
			wait = interval
			continue
		}

		saveMatches(sub, entries, cfg.Keywords)

		idx = (idx + 1) % len(cfg.Subreddits)
		wait = interval
	}
}

func saveMatches(sub string, entries []reddit.Entry, keywords []string) {
	for _, e := range entries {
		if !reddit.MatchesAny(e.Title, keywords) {
			continue
		}
		clip := store.HypeClip{
			PostID:    e.ID,
			Subreddit: sub,
			Title:     e.Title,
			URL:       reddit.OldLink(e.Link.Href),
			Published: e.Published,
		}
		if err := store.SaveClip(&clip); err != nil {
			log.Printf("poller: save clip %s: %v", e.ID, err)
		}
	}
}
