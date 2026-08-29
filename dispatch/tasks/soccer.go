package tasks

import (
	"fmt"

	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

// SoccerNewsDigest delivers any soccer news the background poller has found
// mentioning a configured team. Like SendHypePlays, it never touches the
// network itself — dispatch/poller does that on its own schedule.
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
