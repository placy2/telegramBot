# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A Telegram bot written in Go that aggregates Reddit content across two independent feeds. It polls Telegram for commands via long-polling; a background poller separately searches each feed's configured subreddits for new posts matching that feed's filter, and a command forwards whatever the poller has found for its feed to a specific chat — `/hype` for gaming clips, `/soccer` for configured teams' news.

This is v2 of the bot: the modernization pass is complete. It moved off the abandoned `geddit` library onto unauthenticated Reddit RSS feeds, added the soccer feed alongside the original gaming one, and reorganized the codebase under `dispatch/`. The `dispatch/docs/` planning docs that tracked that work in progress (a modernization runbook, a soccer-digest design doc) have been deleted now that it's done — this file is the current source of truth, not those.

**Reddit's anonymous RSS endpoint is rate-limited to ~1 request/60s, per IP, shared across every feed** — measured directly against `r/GlobalOffensive/new.rss` on 2026-08-29 (see the package comment in `dispatch/poller/poller.go`), not documented behavior. This is why Reddit fetching lives in a background poller (`dispatch/poller`) rather than inline in a command: fetching several subreddits synchronously inside a single command would either 429 or take minutes. If you're debugging "why didn't `/hype` or `/soccer` show anything new," check the poller's tick log before assuming the fetch code is broken.

## Commands

```bash
go build -o bot   # build the bot binary
./bot             # run it (long-polls Telegram for updates)
go vet ./...      # basic static checks
go mod tidy       # keep go.mod/go.sum minimal after dependency changes
```

Tests: `dispatch/reddit/reddit_test.go` covers the two pure functions (`OldLink`, `MatchesAny`). Nothing else in the repo has tests yet.

### Required environment variables

The bot reads secrets from OS environment variables at runtime — there is no `.env` loading:

- `TELEGRAM_KEY` — bot token from BotFather
- `TELEGRAM_OWNER_CHATID` — numeric chat ID that outbound messages (e.g. hype alerts) are sent to

No Reddit credentials are needed — Reddit content is fetched via each subreddit's public `new.rss` feed, unauthenticated.

Everything else tunable (subreddits, keywords/team aliases, post count, poll interval) lives in `dispatch/config/config.json`, not env vars.

## Architecture

Every bot-owned package is nested under `dispatch/` (a deliberate choice — groups everything under one recognizable name without reaching for Go's `internal/`); `main.go` stays at the repo root as the sole entry point.

- `main.go` — entry point. Loads config once, opens a Telegram bot session (`telegram-bot-api/v5`), starts `poller.Run` in a goroutine, long-polls for updates, and dispatches slash commands via a `switch` in the update loop (`/help`, `/start`, `/secretMessage`, `/hype`, `/soccer`). The command's acknowledgment (`bot.Send(msg)`) always fires *before* the command's work function runs, so replies stay in the order a user would expect. Adding a new command means adding a `case` here and (usually) a new function in `dispatch/tasks/`. Shutdown is wired through `signal.NotifyContext` (Ctrl-C / SIGTERM), which cancels the poller's context and calls `bot.StopReceivingUpdates()` — but since `tgbotapi`'s update loop only checks for that between long-polls (each up to 60s, unbounded client-side timeout), the same goroutine force-`os.Exit`s 3s later if the natural shutdown hasn't landed by then. Don't remove the force-exit without fixing the underlying library limitation first.
- `dispatch/reddit/` — `reddit.go`'s `FetchSubreddit(ctx, subreddit, limit)` fetches and parses a subreddit's `new.rss` Atom feed (stdlib `net/http` + `encoding/xml`, no third-party Reddit client), using a `*http.Client` with an explicit 15s timeout (not `http.DefaultClient`, which has none) and a Reddit-conforming `User-Agent`. A non-200 with status 429 comes back as a typed `*RateLimitError` carrying `RetryAfter` (parsed from `x-ratelimit-reset`/`Retry-After`) so callers can back off intelligently instead of guessing. `OldLink` rewrites a `reddit.com` URL to `old.reddit.com` via `net/url` (host rewrite, not string concatenation — the feed's `<link href>` is already an absolute URL). `match.go`'s exported `MatchesAny` is the case-insensitive keyword matcher, shared by `dispatch/tasks` and `dispatch/poller`. Note the RSS endpoint caps at 100 entries per fetch with no working pagination — a single call cannot reach back arbitrarily far in time on a busy subreddit.
- `dispatch/poller/` — `poller.go`'s `Run(ctx, cfg)` is the only thing that calls `reddit.FetchSubreddit`. `buildFeeds` turns `cfg.Gaming`/`cfg.Soccer` into a `[]Feed` (name, subreddits, and a `Match(title) bool` closure — the soccer closure flattens every configured team's name plus aliases into one keyword list for `reddit.MatchesAny`), and `flatten` explodes those into one `job{feed, sub}` per (feed, subreddit) pair. `Run` round-robins one job per tick (`cfg.PollSeconds`, default 75s) — jobs from every feed interleave in a single rotation, since Reddit's rate limit is shared across all subreddits regardless of feed — and persists matches via `store.SavePost`, tagged with the owning feed's name. On a `*reddit.RateLimitError` it waits `RetryAfter` and retries the *same* job next — the index only advances on success or a non-rate-limit error. `Snapshot(feed)` exposes a mutex-guarded, per-feed `Status{LastSuccess, LastSub, LastErr}` (keyed by feed so one feed's error can't bleed into another's report) so tasks like `SendHypePlays` and `SoccerNewsDigest` can tell "this feed is quiet" apart from "the poller can't reach Reddit for it."
- `dispatch/tasks/` — top-level bot behaviors triggered by commands, one exported function per file, none of which do network I/O directly — Reddit fetching stays in the poller. `hype.go`'s `SendHypePlays()` reads up to 20 undelivered `"gaming"`-feed rows via `store.UnsentPosts`, sends each through `telegram.Send`, then `store.MarkSent`s them; `soccer.go`'s `SoccerNewsDigest()` does the same for up to 10 `"soccer"`-feed rows. If there's nothing to send, both fall back to `quiet.go`'s shared `reportQuiet(feed, label, quietMsg)`, which consults `poller.Snapshot(feed)` to pick an honest message — a genuinely quiet feed vs. a stale/broken poller (no successful fetch in the last 10 minutes, `staleAfter`) — rather than always reporting the same generic empty result.
- `dispatch/telegram/` — `telegram.go`'s `Send(message)` opens its own independent `tgbotapi.NewBotAPI` connection (separate from the one in `main.go`) to push a message to `TELEGRAM_OWNER_CHATID` — this is how tasks send unsolicited/outbound messages outside the command-response flow.
- `dispatch/config/` — `config.go`'s `Load(path)` reads and unmarshals `config.json` into a `Config` struct and fills in defaults for anything missing/zero (`NumPosts` → 25, `PollSeconds` → 75, floored at 60 so it can't be tuned back into the rate limit). Per-feed tuning lives in two nested sections: `Gaming` (subreddits + a flat keyword list) and `Soccer` (subreddits + a list of `Team{Name, Aliases}`, flattened by the poller into one keyword list per fetch). Some soccer aliases in `config.json` carry a deliberate trailing space (`"COL "`, `"WHU "`, `"SGE "`) — `reddit.MatchesAny` is a plain substring match, so without it a short alias like `"COL"` would also match unrelated words (e.g. "college"); don't "clean up" that whitespace. `SoccerConfig.LookbackHours` is parsed but not yet read by any code — don't assume it filters anything until something actually consumes it. `main.go` loads the config once at startup and passes it into the poller and tasks as an argument, rather than either reading files or env vars themselves.
- `dispatch/store/` — persistence via GORM over SQLite. `entities.go` defines `FetchedPost` (`Feed` — `"gaming"` or `"soccer"` — plus `PostID` unique-indexed, `Subreddit`, `Title`, `URL`, `Published`, and `SentAt *time.Time` — nil until delivered). `store.go`'s `Init()` opens a **file-backed** SQLite DB (`telegramBot.db`) into a package-level handle with a silent GORM logger, and auto-migrates the schema — state persists across restarts. `Init()` must be called once from `main()` before `SavePost`/`UnsentPosts`/`MarkSent` are used (they operate on the shared package-level handle, not a per-call connection); `main.go` does this at startup. `SavePost` uses `clause.OnConflict{DoNothing: true}` on the unique `PostID` index, so re-fetching a subreddit is naturally idempotent — there's no separate `Exists` check, and only entries that actually matched a keyword ever get persisted. `UnsentPosts(feed, limit)` and `MarkSent(ids)` are scoped by `feed` so, e.g., `/hype` only ever sees gaming rows and never surfaces soccer ones (or vice versa).

## Key behaviors to know when modifying

- Telegram connections are created fresh per call (`tgbotapi.NewBotAPI` in both `main.go` and `dispatch/telegram/telegram.go`) rather than being shared/injected — a known simplicity tradeoff, not an oversight to "fix" incidentally.
- `dispatch/store/store.go`'s `Init()` previously shadowed the package-level `db` with `db, err := gorm.Open(...)` (a local variable, never assigned to the package var) and was never called from `main()` — meaning the old `Exists`/`Create` operated on a permanently-`nil` handle. Fixed by assigning with `=` instead of `:=` and calling `store.Init()` from `main()`. Watch for this shadowing pattern if `Init` is ever refactored.
- Reddit fetching is centralized in `dispatch/poller` specifically because of the measured rate limit above — don't add a second call site that hits `reddit.FetchSubreddit` directly from a command handler, or you'll reintroduce the 429s this was built to avoid. If a new command needs fresh Reddit data, either have the poller also populate the data it needs, or accept the same "read from store" pattern `/hype` and `/soccer` use.
