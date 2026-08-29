package store

import (
	"time"

	"gorm.io/gorm"
)

// HypeClip is a Reddit post the poller found matching a configured keyword.
// The background poller writes these; SendHypePlays reads and delivers them.
// SentAt is nil until the clip has actually been sent to Telegram, so a
// crash between save and send doesn't lose the clip.
type HypeClip struct {
	gorm.Model
	PostID    string `gorm:"uniqueIndex"` // RSS entry <id>, e.g. t3_1vuw4s6
	Subreddit string
	Title     string
	URL       string
	Published time.Time
	SentAt    *time.Time
}
