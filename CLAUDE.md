# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A Telegram bot written in Go that aggregates Reddit content. It polls Telegram for commands via long-polling and, for the `/hype` command, searches a configured list of subreddits for new posts matching configured keywords and forwards matches to a specific chat.

The repo is mid-way through a modernization pass — see `dispatch/docs/modernization-runbook.md` for the staged plan and current status, and `dispatch/docs/daily-soccer-digest.md` for an in-progress second command (`/digest`, not yet implemented — `dispatch/tasks/soccer.go` is currently a placeholder stub). Check those docs before assuming something is finished; of Phase 3's correctness fixes, only hardcoded `bot.Debug` is still open in the code as of this writing — the nil-deref risk in `telegram.Send` and the in-memory-only dedupe DB have both been fixed.

## Commands

```bash
go build -o bot   # build the bot binary
./bot             # run it (long-polls Telegram for updates)
go vet ./...      # basic static checks
go mod tidy       # keep go.mod/go.sum minimal after dependency changes
```

There are no tests in this repository currently.

### Required environment variables

The bot reads secrets from OS environment variables at runtime — there is no `.env` loading:

- `TELEGRAM_KEY` — bot token from BotFather
- `TELEGRAM_OWNER_CHATID` — numeric chat ID that outbound messages (e.g. hype alerts) are sent to

No Reddit credentials are needed — Reddit content is fetched via each subreddit's public `new.rss` feed, unauthenticated.

Everything else tunable (subreddits, keywords, post count) lives in `dispatch/config/config.json`, not env vars.

## Architecture

Every bot-owned package is nested under `dispatch/` (a deliberate choice — groups everything under one recognizable name without reaching for Go's `internal/`); `main.go` stays at the repo root as the sole entry point.

- `main.go` — entry point. Loads config once, opens a Telegram bot session (`telegram-bot-api/v5`), long-polls for updates, and dispatches slash commands via a `switch` in the update loop (`/help`, `/start`, `/secretMessage`, `/hype`). Adding a new command means adding a `case` here and (usually) a new function in `dispatch/tasks/`.
- `dispatch/reddit/` — `reddit.go`'s `FetchSubreddit(ctx, subreddit, limit)` fetches and parses a subreddit's `new.rss` Atom feed (stdlib `net/http` + `encoding/xml`, no third-party Reddit client). `OldLink` rewrites a `reddit.com` URL to `old.reddit.com`. Note the RSS endpoint caps at 100 entries per fetch with no working pagination — a single call cannot reach back arbitrarily far in time on a busy subreddit.
- `dispatch/tasks/` — top-level bot behaviors triggered by commands, one exported function per file. `hype.go`'s `SendHypePlays(ctx, cfg)` is the only implemented task: fetches each configured subreddit, dedupes against `store`, filters titles via `matchesAny`, and pushes matches to Telegram. `match.go` holds `matchesAny` — unexported, shared across task files in this package with no import needed. `soccer.go` is currently a do-nothing placeholder for the planned `/digest` command.
- `dispatch/telegram/` — `telegram.go`'s `Send(message)` opens its own independent `tgbotapi.NewBotAPI` connection (separate from the one in `main.go`) to push a message to `TELEGRAM_OWNER_CHATID` — this is how tasks send unsolicited/outbound messages outside the command-response flow.
- `dispatch/config/` — `config.go`'s `Load(path)` reads and unmarshals `config.json` into a `Config` struct (subreddits, keywords, post count). `main.go` loads it once at startup and passes it into tasks as an argument, rather than tasks reading files or env vars themselves.
- `dispatch/store/` — persistence via GORM over SQLite. `entities.go` defines `RedditPost` (a `PostID` field plus GORM's default model fields) used to dedupe posts already seen — `PostID` holds the RSS entry's `<id>` (e.g. `t3_1vuw4s6`), not a URL, despite the struct's name. `store.go`'s `Init()` opens a **file-backed** SQLite DB (`telegramBot.db`) into a package-level handle and auto-migrates the schema — dedupe state persists across restarts. `Init()` must be called once from `main()` before `Exists`/`Create` are used (they operate on the shared package-level handle, not a per-call connection); `main.go` does this at startup.

## Key behaviors to know when modifying

- Telegram connections are created fresh per call (`tgbotapi.NewBotAPI` in both `main.go` and `dispatch/telegram/telegram.go`) rather than being shared/injected — a known simplicity tradeoff, not an oversight to "fix" incidentally.
- `dispatch/store/store.go`'s `Init()` previously shadowed the package-level `db` with `db, err := gorm.Open(...)` (a local variable, never assigned to the package var) and was never called from `main()` — meaning `Exists`/`Create` operated on a permanently-`nil` handle. Fixed by assigning with `=` instead of `:=` and calling `store.Init()` from `main()`. Watch for this shadowing pattern if `Init` is ever refactored.
- The RSS fetch in `dispatch/reddit` performs real network I/O plus a deliberate `time.Sleep(time.Second)` between subreddits, so a command that fetches multiple subreddits blocks the Telegram update loop for several seconds — nothing breaks (updates just queue), but it's sluggish for back-to-back commands.
