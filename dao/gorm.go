package dao

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Init - Initializes database and returns handler
func Init() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic("failed to connect to database")
	}

	db.AutoMigrate(&RedditPost{})

	return db
}
