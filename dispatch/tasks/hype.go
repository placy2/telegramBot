package tasks

import (
	"fmt"
	"time"

	"github.com/placy2/telegramBot/dispatch/poller"
	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

// maxClipsPerCommand caps how many clips a single /hype reply delivers, so
// a backlog built up while the bot was offline can't dump hundreds of
// messages at once.
const maxClipsPerCommand = 20

// staleAfter is how long since poller's last successful fetch before
// SendHypePlays treats the feed as broken rather than just quiet.
const staleAfter = 10 * time.Minute

// SendHypePlays delivers any gaming clips the background poller has found
// matching a configured keyword. It never touches the network itself —
// dispatch/poller does that on its own schedule to respect Reddit's
// rate limit — so this returns immediately.
func SendHypePlays() {
	clips, err := store.UnsentClips(maxClipsPerCommand)
	if err != nil {
		fmt.Println("store error:", err)
		telegram.Send("Something went wrong looking up hype plays. Check the logs.")
		return
	}

	if len(clips) == 0 {
		reportQuiet()
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

// reportQuiet distinguishes "the feed is genuinely quiet" from "the poller
// can't reach Reddit" using poller's health snapshot, instead of reporting
// every empty result as "not very hype at all" regardless of cause.
func reportQuiet() {
	status := poller.Snapshot()

	switch {
	case status.LastErr != nil:
		telegram.Send(fmt.Sprintf("Couldn't check Reddit for hype plays: %v", status.LastErr))
	case status.LastSuccess.IsZero() || time.Since(status.LastSuccess) > staleAfter:
		telegram.Send("Haven't been able to reach Reddit recently — check back in a bit.")
	default:
		telegram.Send("No hype plays were found recently. Not very hype at all.")
	}
}
