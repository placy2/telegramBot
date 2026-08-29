package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/placy2/telegramBot/dispatch/config"
	"github.com/placy2/telegramBot/dispatch/reddit"
	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

// SendHypePlays finds recent gaming clips with "hype" in the title.
func SendHypePlays(ctx context.Context, cfg *config.Config) {
	var sent int
	for _, sub := range cfg.Subreddits {
		entries, err := reddit.FetchSubreddit(ctx, sub, cfg.NumPosts)
		if err != nil {
			fmt.Println("fetch error:", err)
			continue
		}

		for _, e := range entries {
			if store.Exists(e.ID) {
				continue
			}
			store.Create(e.ID)

			if !matchesAny(e.Title, cfg.Keywords) {
				continue
			}
			telegram.Send(fmt.Sprintf("I found this hype clip for you:\n\n%s\n\n%s",
				e.Title, reddit.OldLink(e.Link.Href)))
			sent++
		}
		time.Sleep(time.Second)
	}

	if sent == 0 {
		telegram.Send("No hype plays were found recently. Not very hype at all.")
	}
}
