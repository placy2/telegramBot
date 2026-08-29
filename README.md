# Telegram Bot
A Telegram bot written in Go that aggregates content from Reddit. Used largely for keeping Go skills up to speed. Initially modeled after https://github.com/masnun/telegram-bot and updated with current toolings and dependencies.

This is only possible thanks to a couple of fantastic go libraries:
* [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api/tree/v5) (v5) handles the interaction with the Telegram API
* [gorm](https://github.com/go-gorm/gorm) is a popular, developer friendly ORM for Golang

Reddit content is fetched via each subreddit's public `new.rss` feed — no Reddit account, API key, or login required.

## Running the bot
This bot pulls secret/user-specific information from 2 OS environment variables. For example, someone on Linux systems using some variant of bash will need to define:

```bash
$ $TELEGRAM_OWNER_CHATID #The numeric chatID for a specific user/group chat. See telegram-bot-api README.
$ $TELEGRAM_KEY #The secret key for the bot being used,
                #obtained from the BotFather upon creation of a new Telegram bot.
```

Subreddits, keywords, and other tunables live in [`dispatch/config/config.json`](dispatch/config/config.json) — edit that file directly rather than passing more env vars.

Once the env vars have been defined, simply run the following two commands to start the bot.

```bash
$ go build -o bot
$ ./bot
```

This will run the bot in command mode. You can then simply send `/start` to `AggregatorBot` on Telegram, which should give you a response back.

## Commands

- `/start` — greets you and points at `/help`
- `/help` — lists available commands
- `/hype` — scrapes the subreddits configured in `config.json`, DMs you any recent post whose title matches a configured keyword

## Documentation

- [`dispatch/docs/modernization-runbook.md`](dispatch/docs/modernization-runbook.md) — the staged cleanup/modernization pass this repo is currently partway through (dependency upgrades, the Reddit RSS migration, correctness fixes, package layout).
- [`dispatch/docs/daily-soccer-digest.md`](dispatch/docs/daily-soccer-digest.md) — implementation guide for an in-progress `/digest` command (not yet built).

### Goal/Updates

10/4: Basic sending of popular posts/specific filtered posts (hardcoded) working.

11/2: Working on abstracted bot command, something like `/getposts 10 subreddit(s) searchTerm(s)` to allow for basic title searching in a specific subreddit. Ideally, it will take 1 or more arguments for subreddit and 0 or more arguments for searchTerms, although multi-term handling will be decided later.

2026: Modernization pass underway — moved off the abandoned `geddit` library (Reddit's API lockdown broke it) onto unauthenticated RSS feeds, bumped dependencies, and reorganized the codebase under `dispatch/`. See the runbook linked above for the full plan and current status.
