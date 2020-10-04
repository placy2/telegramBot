package utils

import (
	"fmt"
	"log"
	"os"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// SendTelegramMessage - sends a message via the telegram bot referenced by TELEGRAM_KEY
func SendTelegramMessage(message string) {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
	if err != nil {
		fmt.Println(err.Error())
	}

	if bot.Self.UserName == "" {
		fmt.Println("Error connecting to Telegram!")
		return
	}

	chatID, _ := strconv.ParseInt(os.Getenv("TELEGRAM_OWNER_CHATID"), 10, 64)
	log.Printf("Going to send %s to chatter ID %d", message, chatID)
	msg := tgbotapi.NewMessage(chatID, message)
	bot.Send(msg)
}
