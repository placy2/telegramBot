package tasks

import (
	"fmt"
	"os"

	"github.com/jzelinskie/geddit"
	"github.com/placy2/telegramBot/dao"
	"github.com/placy2/telegramBot/utils"
)

// PushReddit - Pushes reddit posts to the bot
func PushReddit() {
	session, err := geddit.NewLoginSession(
		os.Getenv("REDDIT_USERNAME"),
		os.Getenv("REDDIT_PASSWORD"),
		"gedditAgent v1",
	)

	if err != nil {
		fmt.Println("Reddit login error: ", err)
		return
	}

	subOpts := geddit.ListingOptions{
		Limit: 15,
	}

	// TODO add error handling
	submissions, _ := session.Frontpage(geddit.DefaultPopularity, subOpts)

	for _, s := range submissions {
		if exists := dao.Exists(s.Permalink); !exists {
			fmt.Printf("Title: %s\nAuthor: %s\n\n", s.Title, s.Author)
			dao.Create(s.Permalink)
			utils.SendTelegramMessage(fmt.Sprintf("%s : https://www.reddit.com/%s", s.Title, s.Permalink))
		} else {
			fmt.Println("Exists: ", s.Permalink)
		}
	}
}
