package store

import (
	"errors"
	"fmt"
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

// Init - Initializes database and returns handler
func Init() {
	var err error
	db, err = gorm.Open(sqlite.Open("telegramBot.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	db.AutoMigrate(&RedditPost{})
}

// Exists - check if RedditPost at PostID exists in db
func Exists(PostID string) bool {
	var post RedditPost

	result := db.First(&post, "post_id = ?", PostID)
	if result.Error != nil {

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false
		}

		// Deliberate fail-open: an unexpected DB error is treated as "already
		// seen" rather than "not seen", so a flaky DB can't cause duplicate
		// sends — the tradeoff is a missed headline instead.
		fmt.Println(result.Error.Error())
	}
	return true
}

// Create - create RedditPost in db
func Create(PostID string) {
	var post = RedditPost{PostID: PostID}
	result := db.Create(&post)
	if result.Error != nil {
		fmt.Println(result.Error.Error())
	}
}
