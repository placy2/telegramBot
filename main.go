package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/placy2/telegramBot/dispatch/config"
	"github.com/placy2/telegramBot/dispatch/poller"
	"github.com/placy2/telegramBot/dispatch/store"
	"github.com/placy2/telegramBot/dispatch/tasks"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	if os.Getenv("TELEGRAM_KEY") == "" {
		log.Fatal("TELEGRAM_KEY environment variable is not set")
	}
	if os.Getenv("TELEGRAM_OWNER_CHATID") == "" {
		log.Fatal("TELEGRAM_OWNER_CHATID environment variable is not set")
	}

	store.Init()

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
	if err != nil {
		log.Fatalf("connecting to Telegram: %v", err)
	}

	cfg, err := config.Load("dispatch/config/config.json")
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bot.Debug = os.Getenv("BOT_DEBUG") != ""

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// The poller owns all Reddit I/O, ticking on its own schedule to stay
	// within Reddit's anonymous rate budget (~1 request/min) — see
	// dispatch/poller. Commands like /hype just read what it has found.
	go poller.Run(ctx, cfg)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		bot.StopReceivingUpdates()

		// tgbotapi's update loop only checks for shutdown between
		// long-poll requests, each of which can block up to u.Timeout
		// (60s) waiting on Telegram — a library limitation, not something
		// fixable from here. Rather than leaving Ctrl-C looking dead for
		// up to a minute, force the exit if the natural shutdown doesn't
		// land quickly; there's no buffered work here that needs a
		// longer graceful drain.
		time.Sleep(3 * time.Second)
		log.Println("forcing exit — Telegram long-poll didn't return in time")
		os.Exit(0)
	}()

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		if update.Message.IsCommand() {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			var work func()
			switch update.Message.Command() {
			case "help":
				msg.Text = "/hype - sends hype plays\n/soccer - sends soccer news for configured teams"
			case "start":
				msg.Text = "type /help for available commands."
			case "secretMessage":
				msg.Text = "Either Parker trusts you or you're code savvy (;"
			case "hype":
				msg.Text = "Looking for hype plays..."
				work = tasks.SendHypePlays
			case "soccer":
				msg.Text = "Looking for soccer news..."
				work = tasks.SoccerNewsDigest
			default:
				msg.Text = "I don't know that command"
			}
			// Send the acknowledgment before doing any work, so it always
			// arrives first regardless of how long the command takes.
			if _, err := bot.Send(msg); err != nil {
				log.Printf("sending reply: %v", err)
			}
			if work != nil {
				work()
			}
		}
	}
}
