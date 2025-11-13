package bot

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"corp-bullshifter/internal/claude"
	"corp-bullshifter/internal/config"
	"corp-bullshifter/internal/ratelimit"
	"corp-bullshifter/internal/storage"
)

// HandleStart handles the /start command
func HandleStart(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	text := "👋 Welcome to the Corporate Bullshifter!\n\n" +
		"Send me any message (in any language), and I'll turn it into a polite, " +
		"professional corporate reply.\n\n" +
		"Perfect for work chats and emails!"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending start message: %v", err)
	}
}

// HandleHelp handles the /help command
func HandleHelp(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	text := "📚 How to use:\n\n" +
		"Simply send me any text message, and I'll rewrite it as a polite, " +
		"concise corporate English reply.\n\n" +
		"Examples:\n" +
		"• Informal messages → Professional tone\n" +
		"• Russian/other languages → English\n" +
		"• Casual chat → Work-appropriate communication\n\n" +
		"Commands:\n" +
		"/start - Welcome message\n" +
		"/help - This help message\n" +
		"/stats - Check your usage statistics"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending help message: %v", err)
	}
}

// HandleStats handles the /stats command
func HandleStats(bot *tgbotapi.BotAPI, message *tgbotapi.Message, limiter *ratelimit.Limiter) {
	userID := message.From.ID
	ctx := context.Background()

	requests, tokens, remaining, err := limiter.GetUsage(ctx, userID)
	if err != nil {
		log.Printf("Error getting usage stats: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Sorry, couldn't retrieve your stats right now.")
		bot.Send(msg)
		return
	}

	timeUntilReset := limiter.GetTimeUntilReset()
	hours := int(timeUntilReset.Hours())
	minutes := int(timeUntilReset.Minutes()) % 60

	text := fmt.Sprintf(
		"📊 Your Usage Statistics\n\n"+
			"Requests today: %d\n"+
			"Tokens used: %d / %d\n"+
			"Remaining: %d tokens\n\n"+
			"Reset in: %dh %dm",
		requests, tokens, config.DailyTokenLimit, remaining, hours, minutes)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending stats message: %v", err)
	}
}

// HandleTextMessage handles regular text messages
func HandleTextMessage(
	bot *tgbotapi.BotAPI,
	message *tgbotapi.Message,
	httpClient *http.Client,
	cfg *config.Config,
	store *storage.Storage,
	limiter *ratelimit.Limiter,
	claudeClient *claude.Client,
) {
	ctx := context.Background()
	userID := message.From.ID

	// Get or create user in database
	user, err := store.GetOrCreateUser(ctx, userID, message.From.UserName, message.From.FirstName, message.From.LastName)
	if err != nil {
		log.Printf("Error getting/creating user: %v", err)
	}

	// Estimate tokens for this request
	estimatedTokens := 500

	// Check rate limit and reserve tokens
	allowed, remaining, err := limiter.CheckAndReserve(ctx, userID, estimatedTokens)
	if err != nil {
		log.Printf("Error checking rate limit: %v", err)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "Sorry, couldn't process your request. Please try again.")
		bot.Send(errorMsg)
		return
	}

	if !allowed {
		timeUntilReset := limiter.GetTimeUntilReset()
		hours := int(timeUntilReset.Hours())
		minutes := int(timeUntilReset.Minutes()) % 60

		limitMsg := fmt.Sprintf(
			"⚠️ Daily limit reached!\n\n"+
				"You've used your daily allocation of %d tokens.\n"+
				"Remaining: %d tokens\n\n"+
				"Your limit will reset in %dh %dm\n"+
				"Use /stats to check your usage.",
			config.DailyTokenLimit, remaining, hours, minutes)
		msg := tgbotapi.NewMessage(message.Chat.ID, limitMsg)
		bot.Send(msg)
		return
	}

	// Show typing indicator
	typingAction := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
	if _, err := bot.Request(typingAction); err != nil {
		log.Printf("Error sending typing action: %v", err)
	}

	// Create context with timeout for Claude API
	apiCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call Claude API
	rewrittenText, inputTokens, outputTokens, err := claudeClient.RewriteToCorporate(apiCtx, message.Text)
	actualTokens := inputTokens + outputTokens

	// Log the usage to database (even if failed)
	usageLog := &storage.UsageLog{
		UserID:          user.ID,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     actualTokens,
		MessagePreview:  truncateString(message.Text, 500),
		ResponsePreview: "",
		Model:           cfg.ClaudeModel,
		Success:         err == nil,
	}

	if err != nil {
		log.Printf("Error calling Claude API: %v", err)

		// Refund estimated tokens since request failed
		if adjErr := limiter.AdjustUsage(ctx, userID, -estimatedTokens); adjErr != nil {
			log.Printf("Error refunding tokens: %v", adjErr)
		}

		// Log failed request
		if logErr := store.LogUsage(ctx, usageLog); logErr != nil {
			log.Printf("Error logging failed usage: %v", logErr)
		}

		errorMsg := tgbotapi.NewMessage(message.Chat.ID,
			"Sorry, I couldn't process your request right now. Please try again later.")
		bot.Send(errorMsg)
		return
	}

	// Adjust usage with actual tokens
	adjustment := actualTokens - estimatedTokens
	if err := limiter.AdjustUsage(ctx, userID, adjustment); err != nil {
		log.Printf("Error adjusting token usage: %v", err)
	}

	// Increment request counter
	if err := limiter.IncrementRequests(ctx, userID); err != nil {
		log.Printf("Error incrementing request count: %v", err)
	}

	// Update usage log with success data
	usageLog.ResponsePreview = truncateString(rewrittenText, 500)
	usageLog.TotalTokens = actualTokens

	// Log successful request to database
	if err := store.LogUsage(ctx, usageLog); err != nil {
		log.Printf("Error logging usage: %v", err)
	}

	log.Printf("User %d (%s) used %d tokens (estimated: %d)", userID, message.From.UserName, actualTokens, estimatedTokens)

	// Send the rewritten text back
	msg := tgbotapi.NewMessage(message.Chat.ID, rewrittenText)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending rewritten message: %v", err)
	}
}

// truncateString safely truncates a string to maxLength
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}
