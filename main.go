package main

import (
	"context"
	"log"
	"os"

	"github.com/placy2/telegramBot/dispatch/config"
	"github.com/placy2/telegramBot/dispatch/tasks"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
	if err != nil {
		log.Panic(err)
	}

	cfg, err := config.Load("dispatch/config/config.json")
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	ctx := context.Background()

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

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
				msg.Text = "Looking for hype plays..."
				tasks.SendHypePlays(ctx, cfg)
			default:
				msg.Text = "I don't know that command"
			}
			bot.Send(msg)
		}
	}
}
