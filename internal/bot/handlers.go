package bot

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"corp-bullshifter/internal/claude"
	"corp-bullshifter/internal/config"
	"corp-bullshifter/internal/ratelimit"
	"corp-bullshifter/internal/storage"
)

const (
	subscriptionPriceUSD            = 5.0
	referenceHaikuBudgetUSD         = 3.0
	haikuInputCostPerMillionTokens  = 0.25
	haikuOutputCostPerMillionTokens = 1.25
	subscriptionDuration            = 30 * 24 * time.Hour
)

func calculateMonthlyTokens() int {
	averageCostPerMillion := (haikuInputCostPerMillionTokens + haikuOutputCostPerMillionTokens) / 2
	if averageCostPerMillion == 0 {
		return 0
	}

	totalMillions := referenceHaikuBudgetUSD / averageCostPerMillion
	return int(math.Round(totalMillions * 1_000_000))
}

func calculateStarPrice(starsPerUSD float64) int {
	if starsPerUSD == 0 {
		return 0
	}

	return int(math.Round(subscriptionPriceUSD * starsPerUSD))
}

func HandleSubscribe(bot *tgbotapi.BotAPI, message *tgbotapi.Message, cfg *config.Config) {
	monthlyTokens := calculateMonthlyTokens()
	starsPrice := calculateStarPrice(cfg.StarsPerUSD)

	description := fmt.Sprintf(
		"Monthly pack: %d tokens (same volume as Claude Haiku 4.5 on a $%.0f budget). Valid 30 days.",
		monthlyTokens, referenceHaikuBudgetUSD,
	)

	prices := tgbotapi.NewLabeledPrice("Monthly pass", starsPrice)
	invoice := tgbotapi.NewInvoice(
		message.Chat.ID,
		"Corporate Bullshifter Monthly",
		description,
		"subscription_payload",
		cfg.TelegramProviderToken,
		"XTR",
		[]tgbotapi.LabeledPrice{prices},
	)

	if _, err := bot.Send(invoice); err != nil {
		log.Printf("Error sending invoice: %v", err)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "Failed to start the purchase flow. Please try again later.")
		bot.Send(errorMsg)
		return
	}

	confirmation := fmt.Sprintf(
		"💫 The plan costs %d Stars (~$%.2f). You'll receive %d tokens for %d days.\n",
		starsPrice, subscriptionPriceUSD, monthlyTokens, int(subscriptionDuration.Hours()/24),
	)
	msg := tgbotapi.NewMessage(message.Chat.ID, confirmation)
	bot.Send(msg)
}

func HandlePreCheckout(bot *tgbotapi.BotAPI, query *tgbotapi.PreCheckoutQuery) {
	response := tgbotapi.PreCheckoutConfig{PreCheckoutQueryID: query.ID, OK: true}
	if _, err := bot.Request(response); err != nil {
		log.Printf("Error confirming pre-checkout: %v", err)
	}
}

func HandleSuccessfulPayment(bot *tgbotapi.BotAPI, message *tgbotapi.Message, store *storage.Storage) {
	ctx := context.Background()

	user, err := store.GetOrCreateUser(ctx, message.From.ID, message.From.UserName, message.From.FirstName, message.From.LastName)
	if err != nil {
		log.Printf("Error ensuring user before subscription: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Payment received, but your profile could not be found. We'll restore access manually.")
		bot.Send(msg)
		return
	}

	monthlyTokens := calculateMonthlyTokens()
	sub, err := store.UpsertSubscription(ctx, user.ID, monthlyTokens, subscriptionDuration)
	if err != nil {
		log.Printf("Error creating subscription: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Payment received, but failed to activate the subscription. We'll fix it soon.")
		bot.Send(msg)
		return
	}

	confirmation := fmt.Sprintf(
		"✅ Subscription activated!\nTokens: %d remaining\nExpires: %s",
		sub.RemainingTokens(), sub.ExpiresAt.Format("2006-01-02"),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, confirmation)
	bot.Send(msg)
}

// HandleStart handles the /start command
func HandleStart(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	text := "👋 Welcome to the Corporate Bullshifter!\n\n" +
		"Send me any message (in any language), and I'll turn it into a polite, " +
		"professional corporate reply.\n\n" +
		"Perfect for work chats and emails!\n\n" +
		"Want more throughput? Subscribe with Telegram Stars to unlock monthly Claude tokens. Use /subscribe."

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
		"/stats - Check your usage statistics\n" +
		"/subscribe - Buy a monthly token pack with Telegram Stars"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending help message: %v", err)
	}
}

// HandleStats handles the /stats command
func HandleStats(bot *tgbotapi.BotAPI, message *tgbotapi.Message, limiter *ratelimit.Limiter, store *storage.Storage) {
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

	subscriptionStatus := "No active subscription. Use /subscribe to unlock more tokens."
	if sub, subErr := store.GetActiveSubscription(ctx, userID); subErr == nil && sub != nil {
		subscriptionStatus = fmt.Sprintf(
			"Subscription active until %s. Tokens left: %d",
			sub.ExpiresAt.Format("2006-01-02"), sub.RemainingTokens(),
		)
	}

	text := fmt.Sprintf(
		"📊 Your Usage Statistics\n\n"+
			"Requests today: %d\n"+
			"Tokens used: %d / %d\n"+
			"Remaining: %d tokens\n\n"+
			"Reset in: %dh %dm\n\n"+
			"%s",
		requests, tokens, config.DailyTokenLimit, remaining, hours, minutes, subscriptionStatus)

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

	// Check subscription status
	var activeSubscription *storage.Subscription
	if sub, subErr := store.GetActiveSubscription(ctx, user.ID); subErr == nil {
		activeSubscription = sub
	} else if subErr != nil {
		log.Printf("Error reading subscription: %v", subErr)
	}

	useSubscription := activeSubscription != nil && activeSubscription.RemainingTokens() >= estimatedTokens

	remaining := 0
	if !useSubscription {
		// Check rate limit and reserve tokens
		var allowed bool
		var err error
		allowed, remaining, err = limiter.CheckAndReserve(ctx, userID, estimatedTokens)
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
					"Use /stats to check your usage or /subscribe for a bigger pool.",
				config.DailyTokenLimit, remaining, hours, minutes)
			msg := tgbotapi.NewMessage(message.Chat.ID, limitMsg)
			bot.Send(msg)
			return
		}
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
		if !useSubscription {
			if adjErr := limiter.AdjustUsage(ctx, userID, -estimatedTokens); adjErr != nil {
				log.Printf("Error refunding tokens: %v", adjErr)
			}
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

	if useSubscription {
		if updatedSub, ok, err := store.ConsumeSubscriptionTokens(ctx, user.ID, actualTokens); err != nil {
			log.Printf("Error consuming subscription tokens: %v", err)
		} else if !ok {
			warning := tgbotapi.NewMessage(message.Chat.ID, "Your subscription tokens were insufficient for this request. Please /subscribe again to refresh your pool.")
			bot.Send(warning)
		} else {
			activeSubscription = updatedSub
		}
	} else {
		// Adjust usage with actual tokens
		adjustment := actualTokens - estimatedTokens
		if err := limiter.AdjustUsage(ctx, userID, adjustment); err != nil {
			log.Printf("Error adjusting token usage: %v", err)
		}
	}

	// Increment request counter for overall stats
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
