# Telegram Bot
A Telegram bot written in Go that aggregates content from Reddit. Used largely for keeping Go skills up to speed. Initially modeled after https://github.com/masnun/telegram-bot and updated with current toolings and dependencies.

## Running the bot
Currently, this bot pulls secret or user-specific information from 4 OS environment variables. For example, someone on Linux systems using some variant of bash will need to define:

```bash
$ $REDDIT_USERNAME #Reddit username used to log in
$ $REDDIT_PASSWORD #Reddit password used to log in
$ $TELEGRAM_OWNER_CHATID #The numeric chatID for a specific user/group chat. See telegram-bot-api README.
$ $TELEGRAM_KEY #The secret key for the bot being used, 
                #obtained from the BotFather upon creation of a new Telegram bot.
```
Once these have been defined, simply run the following two commands to start the bot. 

```bash
$ go build -o bot
$ ./bot
```

This will send the first 15 posts on the reddit user's frontpage to the Telegram chat referenced by `$TELEGRAM_OWNER_CHATID` 
and store them in a temporary SQLite database within the project folder. This database overwrites itself each time this bot is run.

### Goal/Updates

10/4: Basic sending of popular posts working. Future work will be directed at building a test suite and refining the functionality slightly. Can be improved with quasi-TDD from that point.
