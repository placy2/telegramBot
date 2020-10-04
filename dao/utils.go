package dao

import "fmt"

//Exists - check if RedditPost at PermaLink exists in db
//TODO Refactor to use Count rather than First
func Exists(PermaLink string) bool {
	db := Init()
	var post RedditPost
	result := db.First(&post, "perma_link = ?", PermaLink)
	if result.Error != nil {
		msg := result.Error.Error()

		if msg == "record not found" {
			return false
		}

		fmt.Println(msg)
	}
	return true
}

//Create - create RedditPost in db
func Create(PermaLink string) {
	db := Init()
	var post = RedditPost{PermaLink: PermaLink}
	result := db.Create(&post)
	if result.Error != nil {
		fmt.Println(result.Error.Error())
	}
}
