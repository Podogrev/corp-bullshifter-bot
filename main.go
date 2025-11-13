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
}

// ClaudeContentBlock represents a content block in the response
type ClaudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// RewriteToCorporateEnglish calls Claude API to rewrite text into polite corporate English
func RewriteToCorporateEnglish(ctx context.Context, client *http.Client, config Configuration, text string) (string, error) {
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
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", config.ClaudeAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.ClaudeAPIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract the text from the first content block
	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return claudeResp.Content[0].Text, nil
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
		"/help - This help message"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending help message: %v", err)
	}
}

// handleTextMessage handles regular text messages
func handleTextMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, client *http.Client, config Configuration) {
	// Show typing indicator
	typingAction := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
	if _, err := bot.Request(typingAction); err != nil {
		log.Printf("Error sending typing action: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call Claude API
	rewrittenText, err := RewriteToCorporateEnglish(ctx, client, config, message.Text)
	if err != nil {
		log.Printf("Error calling Claude API: %v", err)
		errorMsg := tgbotapi.NewMessage(message.Chat.ID,
			"Sorry, I couldn't process your request right now. Please try again later.")
		if _, err := bot.Send(errorMsg); err != nil {
			log.Printf("Error sending error message: %v", err)
		}
		return
	}

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
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID,
					"Unknown command. Use /help to see available commands.")
				bot.Send(msg)
			}
			continue
		}

		// Handle text messages
		if update.Message.Text != "" {
			go handleTextMessage(bot, update.Message, httpClient, config)
		}
	}
}
