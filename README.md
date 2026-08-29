# Telegram Bot
A Telegram bot written in Go that aggregates content from Reddit. Used largely for keeping Go skills up to speed. Initially modeled after https://github.com/masnun/telegram-bot and updated with current toolings and dependencies.

This is only possible thanks to a couple of fantastic go libraries:
* [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api/tree/v5) (v5) handles the interaction with the Telegram API
* [gorm](https://github.com/go-gorm/gorm) is a popular, developer friendly ORM for Golang

Reddit content is fetched via each subreddit's public `new.rss` feed — no Reddit account, API key, or login required.

Note this RSS change is largely because Reddit has effectively completely blocked hobby bot access, even via OAuth.

**Reddit's anonymous RSS budget is roughly 1 request per 60 seconds, per IP, shared across all feeds** (measured directly). A command can't fetch several subreddits fast enough to reply in real time, so a background poller (`dispatch/poller`) fetches one subreddit per tick on its own schedule and stores keyword matches; `/hype` and `/soccer` just read what's already been found and reply instantly. `pollSeconds` in `config.json` controls the tick interval (default 75s, floored at 60s so it can't be tuned back into 429s).

## Running the bot

### Manual method (no run script, .env defined, or launchd)
This bot pulls secret/user-specific information from 2 OS environment variables. For example, someone on Linux systems using some variant of bash will need to define:

```bash
$ $TELEGRAM_OWNER_CHATID #The numeric chatID for a specific user/group chat. See telegram-bot-api README.
$ $TELEGRAM_KEY #The secret key for the bot being used,
                #obtained from the BotFather upon creation of a new Telegram bot.
```

Subreddits, keywords, and other tunables live in [`dispatch/config/config.json`](dispatch/config/config.json) — edit that file directly rather than passing more env vars.

Set `BOT_DEBUG=1` to log every Telegram update (including full message content); it's off by default.

Once the env vars have been defined, simply run the following two commands to start the bot.

```bash
$ go build -o bot
$ ./bot
```

### Run script && .env
[`run.sh`](run.sh) is a thin wrapper for running the already-built `./bot` binary without exporting env vars in your shell by hand: it sources a `.env.local` file at the repo root, then execs `./bot`. It's the entry point launchd uses below, but you can also run it directly from a terminal.

`.env.local` isn't checked into the repo (it's in `.gitignore` — it holds the same secrets as the manual method above) and needs to be created once per machine. It's just shell variable assignments, e.g.:

```bash
TELEGRAM_KEY=123456:your-bot-token-here
TELEGRAM_OWNER_CHATID=123456789
```

`run.sh` uses `set -a` / `set +a` around the source so every variable defined in the file gets exported automatically — no `export` keyword needed in `.env.local` itself.

Build the binary first (`go build -o bot`), then either run the script directly:

```bash
$ ./run.sh
```

or point launchd at it, below.

### Running under launchd (macOS)

[`com.parkerlacy.telegrambot.plist`](com.parkerlacy.telegrambot.plist) is a launchd agent definition that keeps the bot running in the background without a terminal attached: it runs `run.sh` with `WorkingDirectory` set to the repo root (so `dispatch/config/config.json` and `telegramBot.db` resolve correctly as relative paths), restarts the process if it exits with a nonzero code (`KeepAlive` / `SuccessfulExit: false`), throttles restarts to once every 30 seconds so a persistent crash doesn't spin-loop, and redirects stdout/stderr to `logs/stdout.log` / `logs/stderr.log` since there's no terminal to print to.

Before loading it:

- Build the binary (`go build -o bot`) and create `.env.local` as described above — `run.sh` needs both.
- Copy or symlink the plist into `~/Library/LaunchAgents/` (launchd only looks for per-user agents there), e.g. `ln -s "$(pwd)/com.parkerlacy.telegrambot.plist" ~/Library/LaunchAgents/`.

From there, `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.parkerlacy.telegrambot.plist` (or the older `launchctl load`) registers the agent, `launchctl list | grep telegrambot` confirms it's running and shows its last exit code, and `launchctl bootout gui/$(id -u)/com.parkerlacy.telegrambot` (or `launchctl unload`) stops and deregisters it. Whether you want it to start automatically at login (`RunAtLoad`, already set in the plist) versus start it manually each time is a matter of preference — either way it's the same plist and the same `bootstrap`/`bootout` commands.

This will run the bot in command mode. You can then simply send `/start` to `AggregatorBot` on Telegram, which should give you a response back.

## Commands

- `/start` — greets you and points at `/help`
- `/help` — lists available commands
- `/hype` — DMs you any recent gaming post the background poller has found matching a configured keyword, from the subreddits configured in `config.json`
- `/soccer` — DMs you any recent post the poller has found mentioning one of the configured teams (by name or alias), from the soccer subreddits in `config.json`

## Documentation

Architecture, package layout, and non-obvious behaviors are documented in [`CLAUDE.md`](CLAUDE.md) rather than a separate design doc — the modernization pass it describes is complete, and the planning docs that tracked it in progress have been removed now that the work landed.

### Goal/Updates

10/4: Basic sending of popular posts/specific filtered posts (hardcoded) working.

11/2: Working on abstracted bot command, something like `/getposts 10 subreddit(s) searchTerm(s)` to allow for basic title searching in a specific subreddit. Ideally, it will take 1 or more arguments for subreddit and 0 or more arguments for searchTerms, although multi-term handling will be decided later.

2026: v2 — moved off the abandoned `geddit` library (Reddit's API lockdown broke it) onto unauthenticated RSS feeds, bumped dependencies, reorganized the codebase under `dispatch/`, and added a second feed/command (`/soccer`) alongside the original gaming one (`/hype`).
