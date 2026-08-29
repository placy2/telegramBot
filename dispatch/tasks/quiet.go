package tasks

import (
	"fmt"
	"time"

	"github.com/placy2/telegramBot/dispatch/poller"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

// staleAfter is how long since a feed's last successful fetch before
// reportQuiet treats it as broken rather than just quiet.
const staleAfter = 10 * time.Minute

// reportQuiet distinguishes "this feed is genuinely quiet" from "the poller
// can't reach Reddit for it" using poller's per-feed health snapshot,
// instead of always reporting an empty result the same way regardless of
// cause. label names the feed in the error message (e.g. "hype plays");
// quietMsg is sent when the feed is healthy but has nothing new.
func reportQuiet(feed, label, quietMsg string) {
	status := poller.Snapshot(feed)

	switch {
	case status.LastErr != nil:
		telegram.Send(fmt.Sprintf("Couldn't check Reddit for %s: %v", label, status.LastErr))
	case status.LastSuccess.IsZero() || time.Since(status.LastSuccess) > staleAfter:
		telegram.Send("Haven't been able to reach Reddit recently — check back in a bit.")
	default:
		telegram.Send(quietMsg)
	}
}
