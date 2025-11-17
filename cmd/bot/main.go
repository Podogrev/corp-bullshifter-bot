package main

import (
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"corp-bullshifter/internal/bot"
	"corp-bullshifter/internal/claude"
	"corp-bullshifter/internal/config"
	"corp-bullshifter/internal/ratelimit"
	"corp-bullshifter/internal/storage"
)

func main() {
	log.Println("Starting Corporate Bullshifter bot...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Configuration loaded. Using Claude model: %s", cfg.ClaudeModel)

	// Initialize HTTP client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Initialize Telegram bot
	telegramBot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized on account %s", telegramBot.Self.UserName)

	// Initialize PostgreSQL storage
	store, err := storage.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()
	log.Println("PostgreSQL storage initialized")

	// Initialize Redis rate limiter
	limiter, err := ratelimit.New(cfg.RedisURL, config.DailyTokenLimit)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer limiter.Close()
	log.Printf("Redis rate limiter initialized. Daily limit: %d tokens per user", config.DailyTokenLimit)

	// Initialize Claude API client
	claudeClient := claude.New(cfg.ClaudeAPIKey, cfg.ClaudeAPIURL, cfg.ClaudeModel, httpClient)
	log.Println("Claude API client initialized")

	// Configure update parameters
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates channel
	updates := telegramBot.GetUpdatesChan(u)

	log.Println("Bot is running. Press Ctrl+C to stop.")

	// Process updates
	for update := range updates {
		if update.PreCheckoutQuery != nil {
			go bot.HandlePreCheckout(telegramBot, update.PreCheckoutQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		if update.Message.SuccessfulPayment != nil {
			go bot.HandleSuccessfulPayment(telegramBot, update.Message, store)
			continue
		}

		// Handle commands
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				go bot.HandleStart(telegramBot, update.Message)
			case "help":
				go bot.HandleHelp(telegramBot, update.Message)
			case "stats":
				go bot.HandleStats(telegramBot, update.Message, limiter, store)
			case "subscribe":
				go bot.HandleSubscribe(telegramBot, update.Message, cfg)
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID,
					"Unknown command. Use /help to see available commands.")
				telegramBot.Send(msg)
			}
			continue
		}

		// Handle text messages
		if update.Message.Text != "" {
			go bot.HandleTextMessage(telegramBot, update.Message, httpClient, cfg, store, limiter, claudeClient)
		}
	}
}
