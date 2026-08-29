package tasks

import (
	"fmt"

	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

// maxClipsPerCommand caps how many clips a single /hype reply delivers, so
// a backlog built up while the bot was offline can't dump hundreds of
// messages at once.
const maxClipsPerCommand = 20

// SendHypePlays delivers any gaming clips the background poller has found
// matching a configured keyword. It never touches the network itself —
// dispatch/poller does that on its own schedule to respect Reddit's
// rate limit — so this returns immediately.
func SendHypePlays() {
	clips, err := store.UnsentPosts("gaming", maxClipsPerCommand)
	if err != nil {
		fmt.Println("store error:", err)
		telegram.Send("Something went wrong looking up hype plays. Check the logs.")
		return
	}

	if len(clips) == 0 {
		reportQuiet("gaming", "hype plays", "No hype plays were found recently. Not very hype at all.")
		return
	}

	var sentIDs []uint
	for _, c := range clips {
		telegram.Send(fmt.Sprintf("I found this hype clip for you:\n\n%s\n\n%s", c.Title, c.URL))
		sentIDs = append(sentIDs, c.ID)
	}

	if err := store.MarkSent(sentIDs); err != nil {
		fmt.Println("store error marking sent:", err)
	}
}
