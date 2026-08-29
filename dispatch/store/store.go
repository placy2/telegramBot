package store

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// Init - Initializes database and returns handler
func Init() {
	var err error
	db, err = gorm.Open(sqlite.Open("telegramBot.db"), &gorm.Config{
		// Silent: gorm's default logger prints every "record not found" as a
		// warning, which is expected/routine here (SavePost's OnConflict
		// check, dedupe lookups) rather than an actual problem to surface.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	db.AutoMigrate(&FetchedPost{})
}

// SavePost inserts a FetchedPost, silently doing nothing if PostID already
// exists — this is how re-fetching the same subreddit avoids duplicates
// without a separate Exists check.
func SavePost(c *FetchedPost) error {
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(c)
	return result.Error
}

// UnsentPosts returns up to limit not-yet-delivered posts for the given
// feed, newest first — scoped to one feed so e.g. /hype only ever sees
// gaming posts, not soccer ones the poller saved for /digest.
func UnsentPosts(feed string, limit int) ([]FetchedPost, error) {
	var posts []FetchedPost
	result := db.Where("feed = ? AND sent_at IS NULL", feed).
		Order("published DESC").
		Limit(limit).
		Find(&posts)
	return posts, result.Error
}

// MarkSent stamps the given posts as delivered so UnsentPosts won't return
// them again.
func MarkSent(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	result := db.Model(&FetchedPost{}).Where("id IN ?", ids).Update("sent_at", &now)
	if result.Error != nil {
		fmt.Println(result.Error.Error())
	}
	return result.Error
}
