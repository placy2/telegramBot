package utils

import (
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// SendTelegramMessage - sends a message via the telegram bot referenced by chatID
func SendTelegramMessage(message string, chatID int64) {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
	if err != nil {
		fmt.Println(err.Error())
	}

	if bot.Self.UserName == "" {
		fmt.Println("Error connecting to Telegram!")
		return
	}

	log.Printf("Going to send %s to chatter ID %d", message, chatID)
	msg := tgbotapi.NewMessage(chatID, message)
	bot.Send(msg)
}
