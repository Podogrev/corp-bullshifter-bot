package config

import (
	"fmt"
	"os"
)

// Config holds all application configuration
type Config struct {
	TelegramToken string
	ClaudeAPIKey  string
	ClaudeAPIURL  string
	ClaudeModel   string
	DatabaseURL   string
	RedisURL      string
}

const (
	// DefaultClaudeAPIURL is the default Anthropic API endpoint
	DefaultClaudeAPIURL = "https://api.anthropic.com/v1/messages"
	// DefaultClaudeModel is the default Claude model to use
	DefaultClaudeModel = "claude-3-5-sonnet-20241022"
	// DailyTokenLimit is the maximum tokens per user per day
	DailyTokenLimit = 10000
)

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		ClaudeAPIKey:  os.Getenv("CLAUDE_API_KEY"),
		ClaudeAPIURL:  os.Getenv("CLAUDE_API_URL"),
		ClaudeModel:   os.Getenv("CLAUDE_MODEL"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
	}

	// Validate required fields
	if cfg.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required")
	}
	if cfg.ClaudeAPIKey == "" {
		return nil, fmt.Errorf("CLAUDE_API_KEY environment variable is required")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL environment variable is required")
	}

	// Set defaults for optional fields
	if cfg.ClaudeAPIURL == "" {
		cfg.ClaudeAPIURL = DefaultClaudeAPIURL
	}
	if cfg.ClaudeModel == "" {
		cfg.ClaudeModel = DefaultClaudeModel
	}

	return cfg, nil
}
