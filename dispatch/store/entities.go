package store

import (
	"time"

	"gorm.io/gorm"
)

// FetchedPost is a Reddit post the poller found matching a configured keyword.
// The background poller writes these to the database, and the command handlers read them to send to Telegram.
// SentAt is nil until the post has actually been sent to Telegram, so a
// crash between save and send doesn't lose the post.
type FetchedPost struct {
	gorm.Model
	Feed      string // which configured feed matched this post, e.g. "gaming" or "soccer"
	PostID    string `gorm:"uniqueIndex"` // RSS entry <id>, e.g. t3_1vuw4s6
	Subreddit string
	Title     string
	URL       string
	Published time.Time
	SentAt    *time.Time
}
