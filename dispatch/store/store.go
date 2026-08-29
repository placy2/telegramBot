package store

import (
	"errors"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

// Init - Initializes database and returns handler
func Init() {
	db, err := gorm.Open(sqlite.Open("telegramBot.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect to database")
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
