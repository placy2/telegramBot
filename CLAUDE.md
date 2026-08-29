# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A Telegram bot written in Go that aggregates Reddit content. It polls Telegram for commands via long-polling; a background poller separately searches a configured list of subreddits for new posts matching configured keywords, and the `/hype` command forwards whatever the poller has found to a specific chat.

The repo is mid-way through a modernization pass — see `dispatch/docs/modernization-runbook.md` for the staged plan and current status, and `dispatch/docs/daily-soccer-digest.md` for an in-progress second command (`/digest`, not yet implemented — `dispatch/tasks/soccer.go` is currently a placeholder stub). Check those docs before assuming something is finished. All of Phase 3's correctness fixes are closed now, including the previously-open hardcoded `bot.Debug` (now gated on `BOT_DEBUG`).

**Reddit's anonymous RSS endpoint is rate-limited to ~1 request/60s, per IP, shared across all feeds** — measured directly (see the runbook), not documented behavior. This is why Reddit fetching lives in a background poller (`dispatch/poller`) rather than inline in the `/hype` command: fetching several subreddits synchronously inside a single command would either 429 or take minutes. If you're debugging "why didn't `/hype` show anything new," check the poller's tick log before assuming the fetch code is broken.

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

Everything else tunable (subreddits, keywords, post count, poll interval) lives in `dispatch/config/config.json`, not env vars.

## Architecture

Every bot-owned package is nested under `dispatch/` (a deliberate choice — groups everything under one recognizable name without reaching for Go's `internal/`); `main.go` stays at the repo root as the sole entry point.

- `main.go` — entry point. Loads config once, opens a Telegram bot session (`telegram-bot-api/v5`), starts `poller.Run` in a goroutine, long-polls for updates, and dispatches slash commands via a `switch` in the update loop (`/help`, `/start`, `/secretMessage`, `/hype`). The command's acknowledgment (`bot.Send(msg)`) always fires *before* the command's work function runs, so replies stay in the order a user would expect. Adding a new command means adding a `case` here and (usually) a new function in `dispatch/tasks/`. Shutdown is wired through `signal.NotifyContext` (Ctrl-C / SIGTERM), which cancels the poller's context and calls `bot.StopReceivingUpdates()` — but since `tgbotapi`'s update loop only checks for that between long-polls (each up to 60s, unbounded client-side timeout), the same goroutine force-`os.Exit`s 3s later if the natural shutdown hasn't landed by then. Don't remove the force-exit without fixing the underlying library limitation first.
- `dispatch/reddit/` — `reddit.go`'s `FetchSubreddit(ctx, subreddit, limit)` fetches and parses a subreddit's `new.rss` Atom feed (stdlib `net/http` + `encoding/xml`, no third-party Reddit client), using a `*http.Client` with an explicit 15s timeout (not `http.DefaultClient`, which has none) and a Reddit-conforming `User-Agent`. A non-200 with status 429 comes back as a typed `*RateLimitError` carrying `RetryAfter` (parsed from `x-ratelimit-reset`/`Retry-After`) so callers can back off intelligently instead of guessing. `OldLink` rewrites a `reddit.com` URL to `old.reddit.com` via `net/url` (host rewrite, not string concatenation — the feed's `<link href>` is already an absolute URL). `match.go`'s exported `MatchesAny` is the case-insensitive keyword matcher, shared by `dispatch/tasks` and `dispatch/poller`. Note the RSS endpoint caps at 100 entries per fetch with no working pagination — a single call cannot reach back arbitrarily far in time on a busy subreddit.
- `dispatch/poller/` — `poller.go`'s `Run(ctx, cfg)` is the only thing that calls `reddit.FetchSubreddit`. It round-robins one subreddit per tick (`cfg.PollSeconds`, default 75s), filters entries with `reddit.MatchesAny`, and persists matches via `store.SavePost`. On a `*reddit.RateLimitError` it waits `RetryAfter` and retries the *same* subreddit next — the index only advances on success or a non-rate-limit error. `Snapshot()` exposes a mutex-guarded `Status{LastSuccess, LastSub, LastErr}` so `tasks.SendHypePlays` can tell "the feed is quiet" apart from "the poller can't reach Reddit."
- `dispatch/tasks/` — top-level bot behaviors triggered by commands, one exported function per file. `hype.go`'s `SendHypePlays()` is the only implemented task, and does **no network I/O** — it reads up to 20 undelivered rows via `store.UnsentPosts`, sends each through `telegram.Send`, then `store.MarkSent`s them. If there's nothing to send, it consults `poller.Snapshot()` to pick an honest message (quiet feed vs. stale/broken poller) rather than always reporting "not very hype at all." `soccer.go` is currently a do-nothing placeholder for the planned `/digest` command.
- `dispatch/telegram/` — `telegram.go`'s `Send(message)` opens its own independent `tgbotapi.NewBotAPI` connection (separate from the one in `main.go`) to push a message to `TELEGRAM_OWNER_CHATID` — this is how tasks send unsolicited/outbound messages outside the command-response flow.
- `dispatch/config/` — `config.go`'s `Load(path)` reads and unmarshals `config.json` into a `Config` struct (subreddits, keywords, post count, poll interval) and fills in defaults for anything missing/zero (`NumPosts` → 25, `PollSeconds` → 75, floored at 60 so it can't be tuned back into the rate limit). `main.go` loads it once at startup and passes it into the poller and tasks as an argument, rather than either reading files or env vars themselves.
- `dispatch/store/` — persistence via GORM over SQLite. `entities.go` defines `FetchedPost` (`PostID` unique-indexed, plus `Subreddit`, `Title`, `URL`, `Published`, and `SentAt *time.Time` — nil until delivered). `store.go`'s `Init()` opens a **file-backed** SQLite DB (`telegramBot.db`) into a package-level handle with a silent GORM logger, and auto-migrates the schema — state persists across restarts. `Init()` must be called once from `main()` before `SavePost`/`UnsentPosts`/`MarkSent` are used (they operate on the shared package-level handle, not a per-call connection); `main.go` does this at startup. `SavePost` uses `clause.OnConflict{DoNothing: true}` on the unique `PostID` index, so re-fetching a subreddit is naturally idempotent — there's no separate `Exists` check, and only entries that actually matched a keyword ever get persisted.

## Key behaviors to know when modifying

- Telegram connections are created fresh per call (`tgbotapi.NewBotAPI` in both `main.go` and `dispatch/telegram/telegram.go`) rather than being shared/injected — a known simplicity tradeoff, not an oversight to "fix" incidentally.
- `dispatch/store/store.go`'s `Init()` previously shadowed the package-level `db` with `db, err := gorm.Open(...)` (a local variable, never assigned to the package var) and was never called from `main()` — meaning the old `Exists`/`Create` operated on a permanently-`nil` handle. Fixed by assigning with `=` instead of `:=` and calling `store.Init()` from `main()`. Watch for this shadowing pattern if `Init` is ever refactored.
- Reddit fetching is centralized in `dispatch/poller` specifically because of the measured rate limit above — don't add a second call site that hits `reddit.FetchSubreddit` directly from a command handler, or you'll reintroduce the 429s this was built to avoid. If a new command needs fresh Reddit data, either have the poller also populate the data it needs, or accept the same "read from store" pattern `/hype` uses.
