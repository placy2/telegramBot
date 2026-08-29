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
		// warning, which is expected/routine here (SaveClip's OnConflict
		// check, dedupe lookups) rather than an actual problem to surface.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	db.AutoMigrate(&HypeClip{})
}

// SaveClip inserts a HypeClip, silently doing nothing if PostID already
// exists — this is how re-fetching the same subreddit avoids duplicates
// without a separate Exists check.
func SaveClip(c *HypeClip) error {
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(c)
	return result.Error
}

// UnsentClips returns up to limit not-yet-delivered clips, newest first.
func UnsentClips(limit int) ([]HypeClip, error) {
	var clips []HypeClip
	result := db.Where("sent_at IS NULL").
		Order("published DESC").
		Limit(limit).
		Find(&clips)
	return clips, result.Error
}

// MarkSent stamps the given clips as delivered so UnsentClips won't return
// them again.
func MarkSent(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	result := db.Model(&HypeClip{}).Where("id IN ?", ids).Update("sent_at", &now)
	if result.Error != nil {
		fmt.Println(result.Error.Error())
	}
	return result.Error
}
