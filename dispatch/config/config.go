package config

import (
	"encoding/json"
	"os"
)

// defaultNumPosts and defaultPollSeconds fill in for a missing/zero value
// in config.json — previously a missing "numPosts" silently produced
// limit=0 (an empty fetch) with no error.
const (
	defaultNumPosts    = 25
	defaultPollSeconds = 75

	// minPollSeconds is a hard floor: Reddit's measured anonymous RSS
	// budget is ~1 request/60s/IP (see dispatch/docs/modernization-runbook.md),
	// so anything faster reintroduces the 429s this was built to fix.
	minPollSeconds = 60
)

type Config struct {
	Subreddits  []string `json:"subreddits"`
	Keywords    []string `json:"keywords"`
	NumPosts    int      `json:"numPosts"`
	PollSeconds int      `json:"pollSeconds"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.NumPosts <= 0 {
		cfg.NumPosts = defaultNumPosts
	}
	if cfg.PollSeconds <= 0 {
		cfg.PollSeconds = defaultPollSeconds
	}
	if cfg.PollSeconds < minPollSeconds {
		cfg.PollSeconds = minPollSeconds
	}

	return &cfg, nil
}
