package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Subreddits []string `json:"subreddits"`
	Keywords   []string `json:"keywords"`
	NumPosts   int      `json:"numPosts"`
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
	return &cfg, nil
}
