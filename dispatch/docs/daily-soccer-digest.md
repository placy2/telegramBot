# Implementation guide: daily soccer digest

This is a self-guided build plan, not finished code. It walks through adding a
second bot task — a "digest" of recent Reddit headlines mentioning your
configured soccer teams — alongside the existing `/hype` task, reusing and
expanding the same architecture. Each step names the Go concept it exercises,
points at the existing code it parallels, and gives you a snippet to work
from rather than a drop-in file.

Build incrementally: after each numbered step, run `go build ./...` (or
`go vet ./...`) before moving to the next. A package that doesn't compile in
isolation is much easier to debug than one big change at the end.

## What you're building

- A `/digest` Telegram command that scrapes a configurable list of
  subreddits for recent posts, filters titles for configured team names,
  and DMs you a summary with **old.reddit.com** links. (Read the next
  section before you commit to "recent" meaning 24 hours — it's more
  complicated than it sounds.)
- The subreddit/team config lives in a checked-in JSON file, not env
  vars — easy to tweak without recompiling.
- Deferred: actual cron scheduling (no hosting solution yet). Instead, the
  digest logic gets its own entry point via a `--digest-once` CLI flag, so
  wiring up cron later is just `./bot --digest-once` — no code changes
  needed when that day comes.

## This guide originally targeted `geddit` — that's dead, read this first

This guide was first written against the `geddit` library: Reddit login,
the JSON listing API, cursor-based pagination via `ListingOptions.After`.
None of that survived contact with reality. `geddit`'s login endpoint and
the public JSON API both 403 now (Reddit's 2023 API lockdown), and
personal-use OAuth script-app registration is believed to no longer be
available either. Full investigation is in `modernization-runbook.md`
Phase 1 — short version: the bot now fetches Reddit content via
**unauthenticated Atom/RSS feeds** instead
(`https://www.reddit.com/r/<sub>/new.rss`), and that migration is already
done as part of the general modernization — `dispatch/reddit/reddit.go`'s
`FetchSubreddit` is what you'll build on below.

**One fact from that migration changes this feature's original premise,
so it's worth stating plainly up front:**

- RSS caps at **100 entries per fetch**, and pagination (`after=`) doesn't
  work on this endpoint — confirmed by testing, not documented behavior.
- On a busy subreddit, 100 entries covers roughly **7.6 hours**, not 24.
  Quieter, team-specific subs will cover much more — the number is
  activity-dependent, not fixed.

So a single fetch **cannot** honestly promise "last 24 hours" on a
high-traffic sub. Before you write `tasks/soccer.go`, decide which of
these you actually want, since it changes what `lookbackHours` in your
config means:

1. **Accept "the last ~100 posts" as the practical definition of recent.**
   Reasonable for an on-demand `/digest` you trigger yourself — you're not
   promising a time window, just "what's new."
2. **Run periodically and accumulate.** Each run's dedupe (see Step 5)
   means a digest triggered every few hours effectively builds up the
   24h picture over multiple runs, even though any single fetch doesn't
   cover it. Better long-term, but needs the hosting/cron story this
   project has explicitly deferred.
3. **Point `lookbackHours` at quieter, team-specific subreddits**, where
   100 posts genuinely does span a day or more.

Pick one and note the choice in your `config.json` comments (JSON has no
comments, so a `"_note"` string field or the commit message is the next
best thing) — future-you will otherwise assume the digest promises
something it doesn't.

---

## Steps 1–2 (already done — general cleanup, not soccer-specific)

Two things this guide originally had you build turned out to be needed by
*both* tasks, so they're already part of the modernization pass rather than
something to build here:

- **`reddit.OldLink`** — old.reddit.com link rewriting, in
  `dispatch/reddit/reddit.go`.
- **`matchesAny`** — case-insensitive keyword matching, unexported in
  `dispatch/tasks/match.go` (package `tasks`, so both `hype.go` and your
  new `soccer.go` can call it directly with no import).

Nothing to write here — just read those two files before continuing so
Step 5 makes sense.

## Step 3 — time-window filtering

**Concept:** filtering a slice in place with a `continue`-based skip loop —
about as simple as Go control flow gets, which is the point after the
pagination complexity this step originally had.

The original design paginated through geddit's JSON API via
`ListingOptions.After` to walk backward until a 24h boundary was crossed.
RSS has no working pagination (see above), so there's nothing to walk —
you get one page, and you filter what's in it:

```go
cutoff := time.Now().Add(-lookback)

for _, e := range entries {
	if e.Published.Before(cutoff) {
		continue
	}
	// e is within the window — process it
}
```

`reddit.Entry.Published` is already a `time.Time` (`encoding/xml` parsed
the RFC3339 string for you — see the struct tag in `reddit.go`), so this
is a direct comparison, no timestamp math required.

Worth internalizing: this is simpler code than the pagination version, but
it's simpler *because* the guarantee got weaker (see the section above) —
not a strict improvement. Simple and honest beats complex and only sort of
correct.

## Step 4 — extend the existing config, don't build a new one

**Concept:** growing a struct with a nested sub-config, rather than
standing up a parallel loader — the generic config package already exists
because the modernization pass pulled it out of this guide's original
design for exactly this reuse.

`dispatch/config/config.go` already has `Config` + `Load(path string)
(*Config, error)`, currently shaped for the hype task alone
(`Subreddits`, `Keywords`, `NumPosts`). Add a sibling section rather than
a second package:

```go
type Config struct {
	Subreddits []string     `json:"subreddits"` // existing — hype task
	Keywords   []string     `json:"keywords"`   // existing — hype task
	NumPosts   int          `json:"numPosts"`   // existing — hype task
	Soccer     SoccerConfig `json:"soccer"`     // new
}

type SoccerConfig struct {
	Subreddits    []string `json:"subreddits"`
	Teams         []Team   `json:"teams"`
	LookbackHours int      `json:"lookbackHours"`
}

type Team struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}
```

And the matching addition to `config.json`:

```json
{
  "subreddits": ["VALORANT", "GlobalOffensive", "RocketLeague", "SuperSmashBros"],
  "keywords": ["hype"],
  "numPosts": 15,
  "soccer": {
    "subreddits": ["soccer", "PremierLeague"],
    "teams": [
      { "name": "Arsenal", "aliases": ["Arsenal", "AFC", "Gunners"] },
      { "name": "Liverpool", "aliases": ["Liverpool", "LFC"] }
    ],
    "lookbackHours": 24
  }
}
```

`Team.Aliases` exists because "mentions the team" needs more than an exact
name match — a club has nicknames. `matchesAny` (Steps 1–2) takes a
`[]string`, so this slots right in. One `Load` call in `main.go` now
produces config for both tasks — no second loader, no second file to keep
in sync.

## Step 5 — `tasks/soccer.go`: the task itself

**Concept:** tying the pieces together — this is where you'll spend most of
your thinking time. Structured like `hype.go`'s `SendHypePlays` on
purpose; read that function once more before writing this one. It
currently only has a placeholder in the repo —
`func Soccer(ctx context.Context, subs []string) { _ = ctx; _ = subs }` —
you're replacing that stub, not adding alongside it. Rename it to match
the `SendX` convention `hype.go` already established.

Sketch (fill in the gaps yourself):

```go
package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/placy2/telegramBot/dispatch/config"
	"github.com/placy2/telegramBot/dispatch/reddit"
	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/telegram"
)

func SendSoccerDigest(ctx context.Context, cfg *config.Config) {
	lookback := time.Duration(cfg.Soccer.LookbackHours) * time.Hour
	cutoff := time.Now().Add(-lookback)

	found := map[string][]string{} // team name -> formatted headline lines

	for _, sub := range cfg.Soccer.Subreddits {
		entries, err := reddit.FetchSubreddit(ctx, sub, 100)
		if err != nil {
			fmt.Println("fetch error:", err)
			continue
		}

		for _, e := range entries {
			if e.Published.Before(cutoff) {
				continue
			}
			// TODO: for each cfg.Soccer.Teams, check matchesAny(e.Title, team.Aliases)
			// and append a formatted line (headline + reddit.OldLink(e.Link.Href))
			// to found[team.Name].
		}
		time.Sleep(time.Second)
	}

	// TODO: if found is empty, telegram.Send a "nothing found" message —
	// mirror how SendHypePlays handles its `sent == 0` case.

	// TODO: build the digest with strings.Builder, one section per team,
	// iterating cfg.Soccer.Teams (not the map) so section order is stable —
	// map iteration order is randomized in Go, ranging the config instead
	// of the map is what keeps the digest's team order consistent run to run.

	// TODO: telegram.Send(digest.String())
}
```

Two open decisions the guide won't make for you — think about both before
filling in the TODOs above, and note your answer somewhere (a comment is
fine) since neither has an obviously-correct default:

- **Dedupe.** `hype.go` calls `store.Exists(e.ID)` / `store.Create(e.ID)`
  to never repeat a post — note this uses the RSS entry's `<id>` as the
  key (`store`'s field was recently renamed from `PermaLink` to `PostID`
  to reflect that; see `modernization-runbook.md` Phase 3 if that rename
  hasn't landed yet). Should `/digest` do the same? An on-demand command
  arguably *should* show the full current window every time you ask, even
  if you asked ten minutes ago — which argues against calling `store`
  here at all. If the digest later runs on a schedule instead (see the
  "run periodically and accumulate" option above), you'd want the
  opposite. Decide based on which trigger you're actually building for.
- **Cross-team matches.** If one headline mentions two configured teams
  (a derby, a transfer between them), does it appear under both team
  sections, or just the first match? Either is defensible — pick one and
  the reason (e.g. "appears under both — a derby headline legitimately
  belongs in both digests").

## Step 6 — wire up `main.go`

**Concept:** the `flag` package for CLI arguments, parsed once at
`main()`'s top before anything else happens.

`main.go` already loads config once and passes it into `SendHypePlays`;
follow the same pattern rather than a second `config.Load` call:

```go
digestOnce := flag.Bool("digest-once", false, "run the soccer digest once and exit (for cron use)")
flag.Parse()

// ...after cfg is loaded, before the update loop starts:
if *digestOnce {
	tasks.SendSoccerDigest(ctx, cfg)
	return
}
```

This is the whole point of separating "the task" from "the command
dispatch": the same `SendSoccerDigest(ctx, cfg)` call is reachable from
either entry point with no duplicated logic, and `./bot --digest-once`
never opens the long-polling connection at all.

Then in the existing `switch update.Message.Command()` block, add:
```go
case "digest":
	msg.Text = "Building your digest..."
	tasks.SendSoccerDigest(ctx, cfg)
```
(set `msg.Text` before calling the task — `main.go`'s `"hype"` case
currently forgets to, sending an empty acknowledgment; don't repeat that
one here) and extend the `/help` text to mention `/digest`.

## Step 7 — docs

Small, but don't skip: add a short section to `README.md` documenting
`/digest` and the `soccer` block in `config.json` — genuinely nice
property worth calling out: the bot needs **zero** Reddit credentials at
all now, not just "no new ones." Update `CLAUDE.md`'s architecture notes
to mention `dispatch/tasks/soccer.go` and the `Soccer` config section, so
the next person (or the next Claude session) reading this repo isn't
working from a stale map.

## Verification

- `go build -o bot` after each step — catch compile errors immediately
  rather than after the whole feature is written.
- `go vet ./...` for anything `build` doesn't catch.
- Manual run, with `TELEGRAM_KEY` and `TELEGRAM_OWNER_CHATID` set (the
  only two env vars this needs — no Reddit credentials, since RSS is
  unauthenticated): `./bot --digest-once` should fetch and filter
  entries from each configured subreddit and send (or attempt to send) a
  Telegram message — confirms the flag path short-circuits before the
  update loop.
- Without live Telegram credentials, at minimum confirm
  `SendSoccerDigest` fails gracefully (prints an error, no panic) when a
  fetch or send fails — matching `SendHypePlays`'s existing behavior on
  the same failure paths.
