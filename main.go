package main

import (
	"log"
	"os"
	"strings"

	"github.com/placy2/telegramBot/tasks"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		if update.Message.IsCommand() {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			switch update.Message.Command() {
			case "help":
				msg.Text = "/hype - sends hype plays"
			case "start":
				msg.Text = "type /help for available commands."
			case "secretMessage":
				msg.Text = "Either Parker trusts you or you're code savvy (;"
			case "hype":
				tasks.SendHypePlays(update.Message.Chat.ID)
			case "getPosts":
				updateBody := strings.TrimLeft(update.Message.Text, "/getPosts")
				tasks.GetPosts(updateBody, update.Message.Chat.ID)
			default:
				msg.Text = "I don't know that command"
			}
			bot.Send(msg)
		}
	}
}
