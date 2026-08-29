// Package poller owns all outbound Reddit traffic. Reddit's anonymous RSS
// budget is one request per 60s per IP (measured directly against
// r/GlobalOffensive/new.rss on 2026-08-29 — see
// dispatch/docs/modernization-runbook.md), which is far too slow to serve
// inside a single /hype command across several subreddits. Instead, Run
// ticks in the background, fetching one subreddit per tick and persisting
// keyword matches as store.FetchedPost rows; /hype then just reads the store.
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

// Status is a snapshot of one feed's most recent fetch, used by tasks like
// SendHypePlays to tell a genuinely quiet feed apart from a broken one.
type Status struct {
	LastSuccess time.Time
	LastSub     string
	LastErr     error
}

var (
	mu     sync.Mutex
	status = map[string]Status{}
)

// Snapshot returns the given feed's current health. Safe to call
// concurrently. Feeds are tracked separately — since jobs from every
// configured feed interleave in one rotation, a single global Status would
// let one feed's error or success bleed into another's report.
func Snapshot(feed string) Status {
	mu.Lock()
	defer mu.Unlock()
	return status[feed]
}

func setStatus(feed, sub string, err error) {
	mu.Lock()
	defer mu.Unlock()
	s := status[feed]
	s.LastSub = sub
	s.LastErr = err
	if err == nil {
		s.LastSuccess = time.Now()
	}
	status[feed] = s
}

type Feed struct {
	Name       string // either gaming or soccer right now, persists to the FetchedPost object
	Subreddits []string
	Match      func(string) bool // returns true if the title matches the filter
}

// job is one (feed, subreddit) pair in the round-robin rotation. Feeds are
// flattened into jobs rather than polled feed-by-feed because Reddit's
// anonymous RSS rate limit is shared across every subreddit regardless of
// which feed it belongs to.
type job struct {
	feed Feed
	sub  string
}

// buildFeeds turns cfg's per-command sections into pollable Feeds. Soccer's
// team/alias structure is flattened into a single alias list here.
func buildFeeds(cfg *config.Config) []Feed {
	var feeds []Feed

	if len(cfg.Gaming.Subreddits) > 0 {
		keywords := cfg.Gaming.Keywords
		feeds = append(feeds, Feed{
			Name:       "gaming",
			Subreddits: cfg.Gaming.Subreddits,
			Match: func(title string) bool {
				return reddit.MatchesAny(title, keywords)
			},
		})
	}

	if len(cfg.Soccer.Subreddits) > 0 {
		var all_aliases []string
		for _, t := range cfg.Soccer.Teams {
			all_aliases = append(all_aliases, t.Name)
			all_aliases = append(all_aliases, t.Aliases...)
		}
		feeds = append(feeds, Feed{
			Name:       "soccer",
			Subreddits: cfg.Soccer.Subreddits,
			Match: func(title string) bool {
				return reddit.MatchesAny(title, all_aliases)
			},
		})
	}

	return feeds
}

func flatten(feeds []Feed) []job {
	var jobs []job
	for _, f := range feeds {
		for _, sub := range f.Subreddits {
			jobs = append(jobs, job{feed: f, sub: sub})
		}
	}
	return jobs
}

// Run round-robins one subreddit fetch per tick, across every configured
// feed combined, and persists matches to store.
func Run(ctx context.Context, cfg *config.Config) {
	jobs := flatten(buildFeeds(cfg))
	if len(jobs) == 0 {
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

		j := jobs[idx]
		entries, err := reddit.FetchSubreddit(ctx, j.sub, cfg.NumPosts)
		setStatus(j.feed.Name, j.sub, err)

		if err != nil {
			var rle *reddit.RateLimitError
			if errors.As(err, &rle) {
				// Don't advance idx — retry the same job once the window
				// resets, rather than silently skipping it.
				log.Printf("poller: rate limited on r/%s, retrying in %s", j.sub, rle.RetryAfter)
				wait = rle.RetryAfter
				continue
			}
			log.Printf("poller: fetch error for r/%s: %v", j.sub, err)
			idx = (idx + 1) % len(jobs)
			wait = interval
			continue
		}

		saveMatches(j.feed.Name, j.sub, entries, j.feed.Match)

		idx = (idx + 1) % len(jobs)
		wait = interval
	}
}

func saveMatches(feedName, sub string, entries []reddit.Entry, match func(string) bool) {
	for _, e := range entries {
		if !match(e.Title) {
			continue
		}
		post := store.FetchedPost{
			Feed:      feedName,
			PostID:    e.ID,
			Subreddit: sub,
			Title:     e.Title,
			URL:       reddit.OldLink(e.Link.Href),
			Published: e.Published,
		}
		if err := store.SavePost(&post); err != nil {
			log.Printf("poller: save post %s: %v", e.ID, err)
		}
	}
}
