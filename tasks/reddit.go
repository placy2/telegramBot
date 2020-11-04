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
func SendHypePlays(id int64) {
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
				utils.SendTelegramMessage(fmt.Sprintf("I found this hype clip for you: \n\n%s : \n\nhttps://www.reddit.com/%s", s.Title, s.Permalink), id)
				hypePlaySent = true
			}
		} else {
			fmt.Println("Already exists: ", s.Permalink)
		}
	}
	if !hypePlaySent {
		utils.SendTelegramMessage("No hype plays were found recently. Not very hype at all.", id)
	}
}

// GetPosts - Takes a message body and handles its request, either by providing getPosts usage info or by breaking the string into:
// 						 - number of new posts to search through (numPosts)
//						 - subreddit(s) to pull from (subreddits)
//             - search term (query), if whitespace or missing use no search term
func GetPosts(body string, id int64) {
	if len(body) < 1 || body == "usage" || body == "help" {
		utils.SendTelegramMessage(`To use this command, type /getPosts followed by (separated by spaces):
															 a number of recent posts to search through, a list of subreddit name(s) to search in (comma separated),
															 and (optional) a search term that must be contained in the post title (case sensitive)`, id)
		utils.SendTelegramMessage("Example for searching 3 subreddits for a term: /getPosts 30 news,politics,worldnews election", id)
	} else {
		utils.SendTelegramMessage("Get posts functionality still in development.", id)
	}
}
