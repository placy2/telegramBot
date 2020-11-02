package tasks

import (
	"fmt"
	"os"
	"strings"

	"github.com/jzelinskie/geddit"
	"github.com/placy2/telegramBot/dao"
	"github.com/placy2/telegramBot/utils"
)

// SendHypePlays - Pushes reddit posts from specific gaming subs with 'hype' in the title to the bot
func SendHypePlays() {
	session, err := geddit.NewLoginSession(
		os.Getenv("REDDIT_USERNAME"),
		os.Getenv("REDDIT_PASSWORD"),
		"gedditAgent v1",
	)

	if err != nil {
		fmt.Println("Reddit login error: ", err)
		return
	}

	// Gets new posts in various gaming subreddits and look for the word hype, if found send all "hype posts" on Telegram
	subreddits := []string{"VALORANT", "GlobalOffensive", "RocketLeague", "SuperSmashBros"}
	submissions := utils.GetFromSubreddits(subreddits, 15, session)

	hypePlaySent := false
	for _, s := range submissions {
		if exists := dao.Exists(s.Permalink); !exists {
			fmt.Printf("Title: %s\nAuthor: %s\n\n", s.Title, s.Author)
			dao.Create(s.Permalink)
			if strings.Contains(s.Title, "HYPE") || strings.Contains(s.Title, "Hype") || strings.Contains(s.Title, "hype") {
				utils.SendTelegramMessage(fmt.Sprintf("I found this hype clip for you: \n\n%s : \n\nhttps://www.reddit.com/%s", s.Title, s.Permalink))
				hypePlaySent = true
			}
		} else {
			fmt.Println("Already exists: ", s.Permalink)
		}
	}
	if !hypePlaySent {
		utils.SendTelegramMessage("No hype plays were found recently. Not very hype at all.")
	}
}
