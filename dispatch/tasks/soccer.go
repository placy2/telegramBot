package tasks

import (
	"fmt"

	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

// SoccerNewsDigest is a placeholder exported function for the soccer news digest task.
func SoccerNewsDigest() {
	posts, err := store.UnsentPosts("soccer", 10)
	if err != nil {
		fmt.Println("store error:", err)
		telegram.Send("Something went wrong looking up soccer news. Check the logs.")
		return
	}

	if len(posts) == 0 {
		reportQuiet("soccer", "soccer news", "No soccer news was found recently for the configured teams.")
		return
	}

	var sentIDs []uint
	for _, p := range posts {
		telegram.Send(fmt.Sprintf("Soccer news:\n\n%s\n\n%s", p.Title, p.URL))
		sentIDs = append(sentIDs, p.ID)
	}

	if err := store.MarkSent(sentIDs); err != nil {
		fmt.Println("store error marking sent:", err)
	}
}
