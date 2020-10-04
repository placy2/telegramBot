package dao

import (
	"gorm.io/gorm"
)

// RedditPost - Struct for storing Reddit posts
// Used in ./gorm.go
type RedditPost struct {
	gorm.Model
	PermaLink string
}
