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

  > **Stale premise, flagged 2026-08-29 — read before Step 6.** "No
  > hosting solution yet" was true when this was written; it no longer
  > is. Two things changed it: `dispatch/poller` now runs a periodic
  > in-process fetcher for as long as the bot is up (see the runbook's
  > Phase 1 correction), and the repo picked up `run.sh` + a launchd
  > plist (`com.parkerlacy.telegrambot.plist`, `RunAtLoad`+`KeepAlive`)
  > to keep the bot itself running continuously. Cron-triggering a
  > separate short-lived process is no longer the only way to get
  > periodic behavior — it may not be the best way either, now that a
  > persistent process is the plan. See the note on Step 6 below before
  > building `--digest-once`; this is a real decision, not a stale fact
  > to just correct in place.

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
   cover it. Better long-term — and as of 2026-08-29, this no longer
   needs a hosting/cron story at all: `dispatch/poller` already runs
   periodically, in-process, for as long as the bot is up. Extending it
   to also check team aliases gets you this option for free (see Step 5).
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
- **`reddit.MatchesAny`** — case-insensitive keyword matching, in
  `dispatch/reddit/match.go`. (Briefly lived unexported in
  `dispatch/tasks/match.go`; moved and exported once `dispatch/poller`
  needed to call it too — see the runbook's Phase 4b note if you want the
  history.) Import `dispatch/reddit` to call it from `soccer.go`, same as
  `hype.go` already does.

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
(`Subreddits`, `Keywords`, `NumPosts`, plus `PollSeconds` added for the
poller — don't drop that field when you edit this struct). Add a sibling
section rather than a second package:

```go
type Config struct {
	Subreddits  []string     `json:"subreddits"`  // existing — hype task
	Keywords    []string     `json:"keywords"`    // existing — hype task
	NumPosts    int          `json:"numPosts"`     // existing — hype task
	PollSeconds int          `json:"pollSeconds"`  // existing — poller tick interval
	Soccer      SoccerConfig `json:"soccer"`       // new
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
  "pollSeconds": 75,
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
name match — a club has nicknames. `reddit.MatchesAny` (Steps 1–2) takes a
`[]string`, so this slots right in. One `Load` call in `main.go` now
produces config for both tasks — no second loader, no second file to keep
in sync.

`Load` also fills in defaults for missing/zero fields now (`NumPosts` →
25, `PollSeconds` → 75, floored at 60) — a pattern worth extending to
`SoccerConfig` rather than leaving `LookbackHours: 0` to silently produce
a zero-width window if `soccer` is ever omitted from `config.json`.

## Step 5 — `tasks/soccer.go`: the task itself

**Concept:** tying the pieces together — this is where you'll spend most of
your thinking time. Structured like `hype.go`'s `SendHypePlays` on
purpose; read that function once more before writing this one. It
currently only has a placeholder in the repo —
`func Soccer(ctx context.Context, subs []string) { _ = ctx; _ = subs }` —
you're replacing that stub, not adding alongside it. Rename it to match
the `SendX` convention `hype.go` already established.

> **This sketch is now stale — read before writing `soccer.go`.** It was
> written before `dispatch/poller` existed. `FetchSubreddit` inline plus
> `time.Sleep(time.Second)` was based on the same wrong rate-limit
> assumption `hype.go` originally shipped with: measured directly
> (2026-08-29), Reddit's anonymous RSS budget is ~1 request/60s/IP,
> *shared across all feeds* — not per subreddit. A 1-second sleep 429s on
> every fetch but the first, and `/digest` calling `FetchSubreddit`
> directly would also compete with `/hype`'s poller for that same global
> budget. See `dispatch/docs/modernization-runbook.md`'s Phase 1
> correction and `CLAUDE.md`'s architecture section for the fix that
> landed: all Reddit fetching now goes through one background poller
> (`dispatch/poller.Run`), which persists matches as `store.HypeClip` rows
> for `/hype` to read.
>
> **`/digest` should not add a second poller or a second call site for
> `reddit.FetchSubreddit`.** The cleanest path is extending the existing
> poller to also check `cfg.Soccer.Teams` against each entry it already
> fetches (it's pulling every configured subreddit's entries once per
> sweep regardless) and persist soccer matches alongside hype clips — this
> is exactly the "run periodically and accumulate" option this guide
> lists below, and it now falls out for free instead of needing its own
> infrastructure. `SendSoccerDigest` becomes a store read, the same shape
> as `SendHypePlays`, not a fetch loop. The sketch below is left as a
> reference for the *filtering and formatting* logic (team matching,
> section building) — don't copy its fetch loop or its sleep.

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
			// TODO: for each cfg.Soccer.Teams, check reddit.MatchesAny(e.Title, team.Aliases)
			// and append a formatted line (headline + reddit.OldLink(e.Link.Href))
			// to found[team.Name].
		}
		// Don't reintroduce this sleep — see the note above. Fetching now
		// belongs in dispatch/poller, not here.
		time.Sleep(time.Second)
	}

	// TODO: if found is empty, telegram.Send a "nothing found" message —
	// mirror how SendHypePlays handles its zero-clips case.

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

- **Dedupe.** Stale detail, current shape: `store`'s dedupe-only
  `Exists`/`Create` pair is gone — `hype.go` now calls
  `store.UnsentClips`/`store.MarkSent` against a `HypeClip` table that the
  poller populates (`PostID` is still the RSS entry `<id>`, just on a
  richer row now — see `CLAUDE.md`). The underlying question is unchanged
  though: should `/digest` do the same? An on-demand command arguably
  *should* show the full current window every time you ask, even if you
  asked ten minutes ago — which argues against a sent/unsent split here.
  If the digest runs on a schedule instead (see "run periodically and
  accumulate" above — now how the poller already works), you'd want the
  opposite. Decide based on which trigger you're actually building for.
- **Cross-team matches.** If one headline mentions two configured teams
  (a derby, a transfer between them), does it appear under both team
  sections, or just the first match? Either is defensible — pick one and
  the reason (e.g. "appears under both — a derby headline legitimately
  belongs in both digests").

## Step 6 — wire up `main.go`

> **Decide this before writing any of Step 6 — the premise changed.**
> `--digest-once` existed to solve one problem: this bot had no way to
> run periodically, so an external cron job invoking a short-lived
> `./bot --digest-once` process was the only path to "send me a digest
> without me asking for it." As of 2026-08-29 that problem has two
> independent fixes already in place, neither built for this reason but
> both applicable: `dispatch/poller` runs in-process for as long as the
> bot is up, and `run.sh` + `com.parkerlacy.telegrambot.plist`
> (`RunAtLoad`/`KeepAlive`) keep the bot itself running continuously via
> launchd. If the bot is now always up, "no hosting solution" is no
> longer true, and a cron-triggered separate process is solving a problem
> that's already solved a different way.
>
> Two real options, not a stale fact to just correct:
>
> - **A — drop `--digest-once` entirely (recommended).** Extend the
>   poller to also check `cfg.Soccer.Teams` against entries it's already
>   fetching, and persist matches the same way `saveMatches` does for
>   hype clips. `/digest` becomes a Telegram command that reads the
>   store — no network call, no `flag` package, no second entry point.
>   Same shape as `/hype`, and if you want it pushed automatically rather
>   than asked-for, that's a small ticker in `main.go` (or in the poller)
>   that calls the send function on a schedule — still one persistent
>   process, no cron. Simpler, and consistent with how `/hype` already
>   works.
> - **B — keep `--digest-once`, but as a genuinely separate mode.** Valid
>   if you want the digest runnable *without* the long-lived bot process
>   at all — e.g. from a machine that isn't running the launchd job, or
>   if you decide you don't trust `KeepAlive` and want cron as a
>   independent fallback trigger. In that case `--digest-once` needs to
>   do its own one-shot fetch (roughly the original sketch below, minus
>   the bad sleep) rather than reading a store the poller may not have
>   populated in a separate process invocation — the two don't share
>   state across processes.
>
> The rest of this step assumes B, since that's what it originally
> documented — adapt to A's shape (a `case "digest":` in the switch,
> backed by a store read like `hype.go`'s, no `flag` code at all) if you
> go that way, which is the smaller amount of code either way.

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
	work = func() { tasks.SendSoccerDigest(ctx, cfg) }
```
Match `main.go`'s current shape here, not an older version of it: `msg.Text`
is set inline (the empty-ack bug this section used to warn about is fixed),
but the task call goes into the `work` closure and runs *after*
`bot.Send(msg)`, not inline in the case — that's what makes the ack arrive
before the digest does, the same fix `/hype` needed once
`SendHypePlays`/`SendSoccerDigest` do enough work to take noticeable time.
See `main.go`'s current `case "hype":` for the exact pattern to copy. Also
extend the `/help` text to mention `/digest`.

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
  unauthenticated):
  - **If you built Option B** (`--digest-once`): `./bot --digest-once`
    should fetch and filter entries from each configured subreddit and
    send (or attempt to send) a Telegram message — confirms the flag path
    short-circuits before the update loop.
  - **If you built Option A** (recommended — no flag): send `/digest` in
    Telegram after the bot's been running long enough for the poller to
    have swept the configured soccer subreddits at least once (poll
    interval × subreddit count, worst case), and confirm the ack arrives
    before the digest, same check as `/hype`.
- Without live Telegram credentials, at minimum confirm
  `SendSoccerDigest` fails gracefully (prints an error, no panic) when a
  fetch or send fails (Option B) or when the store read errors (Option A)
  — matching `SendHypePlays`'s existing behavior on its equivalent paths.
