package telegram

import (
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Send - sends a message to chatID via the telegram bot referenced by TELEGRAM_KEY
func Send(chatID int64, message string) {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	if bot.Self.UserName == "" {
		fmt.Println("Error connecting to Telegram!")
		return
	}

	log.Printf("Going to send %s to chatter ID %d", message, chatID)
	msg := tgbotapi.NewMessage(chatID, message)
	bot.Send(msg)
}
