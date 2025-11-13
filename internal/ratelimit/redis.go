package ratelimit

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter handles rate limiting using Redis
type Limiter struct {
	client     *redis.Client
	dailyLimit int
}

// New creates a new Limiter instance
func New(redisURL string, dailyLimit int) (*Limiter, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Println("Successfully connected to Redis")

	return &Limiter{
		client:     client,
		dailyLimit: dailyLimit,
	}, nil
}

// Close closes the Redis connection
func (l *Limiter) Close() error {
	return l.client.Close()
}

// getDateKey generates a Redis key for the current date
func (l *Limiter) getDateKey() string {
	return time.Now().Format("2006-01-02")
}

// getTokenKey generates a Redis key for token usage
func (l *Limiter) getTokenKey(telegramID int64) string {
	return fmt.Sprintf("user:%d:tokens:%s", telegramID, l.getDateKey())
}

// getRequestKey generates a Redis key for request count
func (l *Limiter) getRequestKey(telegramID int64) string {
	return fmt.Sprintf("user:%d:requests:%s", telegramID, l.getDateKey())
}

// CheckAndReserve checks if user can make a request and reserves tokens
// Returns: (allowed, remaining tokens, error)
func (l *Limiter) CheckAndReserve(ctx context.Context, telegramID int64, estimatedTokens int) (bool, int, error) {
	tokenKey := l.getTokenKey(telegramID)

	// Get current usage
	currentTokens, err := l.client.Get(ctx, tokenKey).Int()
	if err != nil && err != redis.Nil {
		return false, 0, fmt.Errorf("failed to get token usage: %w", err)
	}

	// Check if adding estimated tokens would exceed limit
	if currentTokens+estimatedTokens > l.dailyLimit {
		remaining := l.dailyLimit - currentTokens
		if remaining < 0 {
			remaining = 0
		}
		return false, remaining, nil
	}

	// Reserve tokens
	newTotal, err := l.client.IncrBy(ctx, tokenKey, int64(estimatedTokens)).Result()
	if err != nil {
		return false, 0, fmt.Errorf("failed to reserve tokens: %w", err)
	}

	// Set expiration to 48 hours (to keep for next day too)
	l.client.Expire(ctx, tokenKey, 48*time.Hour)

	remaining := l.dailyLimit - int(newTotal)
	if remaining < 0 {
		remaining = 0
	}

	return true, remaining, nil
}

// AdjustUsage adjusts the token usage (positive or negative adjustment)
func (l *Limiter) AdjustUsage(ctx context.Context, telegramID int64, adjustment int) error {
	tokenKey := l.getTokenKey(telegramID)

	_, err := l.client.IncrBy(ctx, tokenKey, int64(adjustment)).Result()
	if err != nil {
		return fmt.Errorf("failed to adjust token usage: %w", err)
	}

	// Ensure expiration is set
	l.client.Expire(ctx, tokenKey, 48*time.Hour)

	return nil
}

// IncrementRequests increments the request counter
func (l *Limiter) IncrementRequests(ctx context.Context, telegramID int64) error {
	requestKey := l.getRequestKey(telegramID)

	_, err := l.client.Incr(ctx, requestKey).Result()
	if err != nil {
		return fmt.Errorf("failed to increment request count: %w", err)
	}

	// Set expiration to 48 hours
	l.client.Expire(ctx, requestKey, 48*time.Hour)

	return nil
}

// GetUsage retrieves current usage statistics
// Returns: (request count, tokens used, remaining tokens)
func (l *Limiter) GetUsage(ctx context.Context, telegramID int64) (int, int, int, error) {
	tokenKey := l.getTokenKey(telegramID)
	requestKey := l.getRequestKey(telegramID)

	// Get tokens used
	tokensUsed, err := l.client.Get(ctx, tokenKey).Int()
	if err != nil && err != redis.Nil {
		return 0, 0, 0, fmt.Errorf("failed to get token usage: %w", err)
	}

	// Get request count
	requestCount, err := l.client.Get(ctx, requestKey).Int()
	if err != nil && err != redis.Nil {
		return 0, 0, 0, fmt.Errorf("failed to get request count: %w", err)
	}

	remaining := l.dailyLimit - tokensUsed
	if remaining < 0 {
		remaining = 0
	}

	return requestCount, tokensUsed, remaining, nil
}

// ResetUserUsage resets usage for a specific user (for testing/admin purposes)
func (l *Limiter) ResetUserUsage(ctx context.Context, telegramID int64) error {
	tokenKey := l.getTokenKey(telegramID)
	requestKey := l.getRequestKey(telegramID)

	pipe := l.client.Pipeline()
	pipe.Del(ctx, tokenKey)
	pipe.Del(ctx, requestKey)
	_, err := pipe.Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to reset user usage: %w", err)
	}

	log.Printf("Reset usage for user %d", telegramID)
	return nil
}

// GetTimeUntilReset returns duration until midnight (reset time)
func (l *Limiter) GetTimeUntilReset() time.Duration {
	now := time.Now()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return tomorrow.Sub(now)
}
