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
	// budget is ~1 request/60s/IP (see dispatch/poller's package comment),
	// so anything faster reintroduces the 429s this was built to fix.
	minPollSeconds = 60
)

type Config struct {
	NumPosts    int          `json:"numPosts"`
	PollSeconds int          `json:"pollSeconds"`
	Gaming      GamingConfig `json:"gaming"`
	Soccer      SoccerConfig `json:"soccer"`
}

// Specific to the '/hype' command originally in this bot - expects subreddits to poll and keywords to filter for in the title of posts.
type GamingConfig struct {
	Subreddits []string `json:"subreddits"`
	Keywords   []string `json:"keywords"`
}

type SoccerConfig struct {
	Subreddits    []string `json:"subreddits"`
	Teams         []Team   `json:"teams"`
	LookbackHours int      `json:"lookbackHours"`
}

type Team struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
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
