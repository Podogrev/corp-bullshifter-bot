package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// Default Claude API endpoint
	defaultClaudeAPIURL = "https://api.anthropic.com/v1/messages"
	// Default Claude model
	defaultClaudeModel = "claude-3-5-sonnet-20241022"
	// Anthropic API version
	anthropicVersion = "2023-06-01"
	// Daily token limit per user
	dailyTokenLimit = 10000
)

// Configuration holds all required settings
type Configuration struct {
	TelegramToken string
	ClaudeAPIKey  string
	ClaudeAPIURL  string
	ClaudeModel   string
}

// ClaudeRequest represents the request body for Claude API
type ClaudeRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []ClaudeMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
}

// ClaudeMessage represents a message in the conversation
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeResponse represents the response from Claude API
type ClaudeResponse struct {
	ID      string               `json:"id"`
	Type    string               `json:"type"`
	Role    string               `json:"role"`
	Content []ClaudeContentBlock `json:"content"`
	Model   string               `json:"model"`
	Usage   ClaudeUsage          `json:"usage"`
}

// ClaudeContentBlock represents a content block in the response
type ClaudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ClaudeUsage represents token usage information from Claude API
type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// UserUsage tracks daily token usage for a user
type UserUsage struct {
	UserID       int64
	RequestCount int
	TokensUsed   int
	LastReset    time.Time
}

// UsageTracker manages user usage tracking
type UsageTracker struct {
	mu    sync.RWMutex
	users map[int64]*UserUsage
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		users: make(map[int64]*UserUsage),
	}
}

// CheckAndUpdateUsage checks if user can make a request and updates usage
func (ut *UsageTracker) CheckAndUpdateUsage(userID int64, tokensUsed int) (allowed bool, remaining int) {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	// Get or create user usage
	usage, exists := ut.users[userID]
	if !exists {
		usage = &UserUsage{
			UserID:    userID,
			LastReset: time.Now(),
		}
		ut.users[userID] = usage
	}

	// Check if we need to reset (new day)
	if time.Since(usage.LastReset) >= 24*time.Hour {
		usage.RequestCount = 0
		usage.TokensUsed = 0
		usage.LastReset = time.Now()
	}

	// Check if user would exceed limit
	if usage.TokensUsed+tokensUsed > dailyTokenLimit {
		return false, dailyTokenLimit - usage.TokensUsed
	}

	// Update usage
	usage.RequestCount++
	usage.TokensUsed += tokensUsed

	return true, dailyTokenLimit - usage.TokensUsed
}

// GetUsage returns current usage for a user
func (ut *UsageTracker) GetUsage(userID int64) (requests int, tokens int, remaining int) {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	usage, exists := ut.users[userID]
	if !exists {
		return 0, 0, dailyTokenLimit
	}

	// Check if we need to reset
	if time.Since(usage.LastReset) >= 24*time.Hour {
		return 0, 0, dailyTokenLimit
	}

	return usage.RequestCount, usage.TokensUsed, dailyTokenLimit - usage.TokensUsed
}

// ResetAllUsage resets usage for all users (called daily)
func (ut *UsageTracker) ResetAllUsage() {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	now := time.Now()
	for _, usage := range ut.users {
		usage.RequestCount = 0
		usage.TokensUsed = 0
		usage.LastReset = now
	}
	log.Println("Daily usage reset completed for all users")
}

// RewriteToCorporateEnglish calls Claude API to rewrite text into polite corporate English
func RewriteToCorporateEnglish(ctx context.Context, client *http.Client, config Configuration, text string) (string, int, error) {
	// Construct the prompt for corporate rewriting
	prompt := fmt.Sprintf(`You are a corporate communication assistant.

Task:
- Rewrite the user's message into a polite, professional reply suitable for workplace communication.
- The reply should sound natural and human, like a real person writing in a work chat or email - not robotic or overly formal.
- Preserve the original meaning and intent, but adjust the tone to be neutral and appropriate for professional settings.
- IMPORTANT: Detect the language of the user's message and respond in the SAME language (e.g., if the message is in Russian, respond in Russian; if in English, respond in English).
- Avoid slang, sarcasm, offensive language, or overly emotional expressions.
- Keep it conversational and friendly, but professional. Use natural phrasing that sounds like how colleagues actually talk to each other.
- Keep the response focused and reasonably short (a few sentences unless the input is clearly long and detailed).
- Output ONLY the final rewritten text, without any preamble, explanation, or commentary. Do not add phrases like "Here's a professional version" or "I apologize" - just output the rewritten text directly.

User message:
%s`, text)

	// Prepare the request body
	reqBody := ClaudeRequest{
		Model:     config.ClaudeModel,
		MaxTokens: 1024,
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", config.ClaudeAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.ClaudeAPIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract the text from the first content block
	if len(claudeResp.Content) == 0 {
		return "", 0, fmt.Errorf("no content in response")
	}

	// Calculate total tokens used
	totalTokens := claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens

	return claudeResp.Content[0].Text, totalTokens, nil
}

// handleStart handles the /start command
func handleStart(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	text := "👋 Welcome to the Corporate Bullshifter!\n\n" +
		"Send me any message (in any language), and I'll turn it into a polite, " +
		"professional corporate reply.\n\n" +
		"Perfect for work chats and emails!"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending start message: %v", err)
	}
}

// handleHelp handles the /help command
func handleHelp(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
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

// handleStats handles the /stats command
func handleStats(bot *tgbotapi.BotAPI, message *tgbotapi.Message, tracker *UsageTracker) {
	userID := message.From.ID
	requests, tokens, remaining := tracker.GetUsage(userID)

	text := fmt.Sprintf(
		"📊 Your Usage Statistics\n\n"+
			"Requests today: %d\n"+
			"Tokens used: %d / %d\n"+
			"Remaining: %d tokens\n\n"+
			"Usage resets every 24 hours.",
		requests, tokens, dailyTokenLimit, remaining)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending stats message: %v", err)
	}
}

// handleTextMessage handles regular text messages
func handleTextMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, client *http.Client, config Configuration, tracker *UsageTracker) {
	userID := message.From.ID

	// Estimate tokens for this request (rough estimate: ~500 tokens)
	// We'll update with actual usage after API call
	estimatedTokens := 500

	// Check if user has enough tokens remaining
	allowed, remaining := tracker.CheckAndUpdateUsage(userID, estimatedTokens)
	if !allowed {
		limitMsg := fmt.Sprintf(
			"⚠️ Daily limit reached!\n\n"+
				"You've used your daily allocation of %d tokens.\n"+
				"Remaining: %d tokens\n\n"+
				"Your limit will reset in 24 hours. Use /stats to check your usage.",
			dailyTokenLimit, remaining)
		msg := tgbotapi.NewMessage(message.Chat.ID, limitMsg)
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Error sending limit message: %v", err)
		}
		return
	}

	// Show typing indicator
	typingAction := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
	if _, err := bot.Request(typingAction); err != nil {
		log.Printf("Error sending typing action: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call Claude API
	rewrittenText, actualTokens, err := RewriteToCorporateEnglish(ctx, client, config, message.Text)
	if err != nil {
		log.Printf("Error calling Claude API: %v", err)

		// Refund estimated tokens since request failed
		tracker.CheckAndUpdateUsage(userID, -estimatedTokens)

		errorMsg := tgbotapi.NewMessage(message.Chat.ID,
			"Sorry, I couldn't process your request right now. Please try again later.")
		if _, err := bot.Send(errorMsg); err != nil {
			log.Printf("Error sending error message: %v", err)
		}
		return
	}

	// Adjust usage with actual tokens (subtract estimate, add actual)
	adjustment := actualTokens - estimatedTokens
	tracker.CheckAndUpdateUsage(userID, adjustment)

	log.Printf("User %d used %d tokens (estimated: %d)", userID, actualTokens, estimatedTokens)

	// Send the rewritten text back
	msg := tgbotapi.NewMessage(message.Chat.ID, rewrittenText)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending rewritten message: %v", err)
	}
}

// loadConfiguration loads configuration from environment variables
func loadConfiguration() (Configuration, error) {
	config := Configuration{
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		ClaudeAPIKey:  os.Getenv("CLAUDE_API_KEY"),
		ClaudeAPIURL:  os.Getenv("CLAUDE_API_URL"),
		ClaudeModel:   os.Getenv("CLAUDE_MODEL"),
	}

	// Validate required fields
	if config.TelegramToken == "" {
		return config, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required")
	}
	if config.ClaudeAPIKey == "" {
		return config, fmt.Errorf("CLAUDE_API_KEY environment variable is required")
	}

	// Set defaults for optional fields
	if config.ClaudeAPIURL == "" {
		config.ClaudeAPIURL = defaultClaudeAPIURL
	}
	if config.ClaudeModel == "" {
		config.ClaudeModel = defaultClaudeModel
	}

	return config, nil
}

// startDailyResetTimer starts a goroutine that resets usage at midnight
func startDailyResetTimer(tracker *UsageTracker) {
	go func() {
		for {
			// Calculate time until next midnight
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			duration := next.Sub(now)

			log.Printf("Next usage reset scheduled in %v", duration)

			// Sleep until midnight
			time.Sleep(duration)

			// Reset all usage
			tracker.ResetAllUsage()
		}
	}()
}

func main() {
	log.Println("Starting Corporate Bullshifter bot...")

	// Load configuration
	config, err := loadConfiguration()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Configuration loaded. Using Claude model: %s", config.ClaudeModel)

	// Initialize HTTP client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Initialize Telegram bot
	bot, err := tgbotapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Initialize usage tracker
	usageTracker := NewUsageTracker()
	log.Printf("Usage tracker initialized. Daily limit: %d tokens per user", dailyTokenLimit)

	// Start daily reset timer
	startDailyResetTimer(usageTracker)

	// Configure update parameters
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates channel
	updates := bot.GetUpdatesChan(u)

	log.Println("Bot is running. Press Ctrl+C to stop.")

	// Process updates
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Handle commands
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				go handleStart(bot, update.Message)
			case "help":
				go handleHelp(bot, update.Message)
			case "stats":
				go handleStats(bot, update.Message, usageTracker)
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID,
					"Unknown command. Use /help to see available commands.")
				bot.Send(msg)
			}
			continue
		}

		// Handle text messages
		if update.Message.Text != "" {
			go handleTextMessage(bot, update.Message, httpClient, config, usageTracker)
		}
	}
}
