package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Storage handles PostgreSQL database operations
type Storage struct {
	pool *pgxpool.Pool
}

// User represents a Telegram user
type User struct {
	ID         int64
	TelegramID int64
	Username   string
	FirstName  string
	LastName   string
	CreatedAt  time.Time
	LastActive time.Time
}

// UsageLog represents a single API request log entry
type UsageLog struct {
	ID              int64
	UserID          int64
	Timestamp       time.Time
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	MessagePreview  string
	ResponsePreview string
	Model           string
	Success         bool
}

// Subscription represents a paid monthly token package
type Subscription struct {
	ID            int64
	UserID        int64
	ExpiresAt     time.Time
	TokensGranted int
	TokensUsed    int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// New creates a new Storage instance and connects to PostgreSQL
func New(databaseURL string) (*Storage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Connection pool settings
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Successfully connected to PostgreSQL")

	return &Storage{pool: pool}, nil
}

// Close closes the database connection pool
func (s *Storage) Close() {
	s.pool.Close()
	log.Println("PostgreSQL connection pool closed")
}

// GetOrCreateUser retrieves an existing user or creates a new one
func (s *Storage) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lastName string) (*User, error) {
	user := &User{}

	// Try to get existing user
	query := `
		SELECT id, telegram_id, username, first_name, last_name, created_at, last_active
		FROM users
		WHERE telegram_id = $1
	`
	err := s.pool.QueryRow(ctx, query, telegramID).Scan(
		&user.ID, &user.TelegramID, &user.Username, &user.FirstName,
		&user.LastName, &user.CreatedAt, &user.LastActive,
	)

	if err == nil {
		// User exists, update last_active and username if changed
		updateQuery := `
			UPDATE users
			SET last_active = CURRENT_TIMESTAMP,
			    username = $1,
			    first_name = $2,
			    last_name = $3
			WHERE telegram_id = $4
		`
		_, err = s.pool.Exec(ctx, updateQuery, username, firstName, lastName, telegramID)
		if err != nil {
			log.Printf("Warning: failed to update user last_active: %v", err)
		}
		return user, nil
	}

	// User doesn't exist, create new
	insertQuery := `
		INSERT INTO users (telegram_id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		RETURNING id, telegram_id, username, first_name, last_name, created_at, last_active
	`
	err = s.pool.QueryRow(ctx, insertQuery, telegramID, username, firstName, lastName).Scan(
		&user.ID, &user.TelegramID, &user.Username, &user.FirstName,
		&user.LastName, &user.CreatedAt, &user.LastActive,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("Created new user: telegram_id=%d, username=%s", telegramID, username)
	return user, nil
}

// LogUsage records an API request in the database
func (s *Storage) LogUsage(ctx context.Context, log *UsageLog) error {
	query := `
		INSERT INTO usage_logs (
			user_id, input_tokens, output_tokens, total_tokens,
			message_preview, response_preview, model, success
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, timestamp
	`

	err := s.pool.QueryRow(ctx, query,
		log.UserID, log.InputTokens, log.OutputTokens, log.TotalTokens,
		log.MessagePreview, log.ResponsePreview, log.Model, log.Success,
	).Scan(&log.ID, &log.Timestamp)

	if err != nil {
		return fmt.Errorf("failed to log usage: %w", err)
	}

	return nil
}

// GetDailyUsage retrieves daily usage statistics for a user
func (s *Storage) GetDailyUsage(ctx context.Context, telegramID int64, date time.Time) (requestCount int, totalTokens int, err error) {
	query := `SELECT * FROM get_user_daily_usage($1, $2)`

	err = s.pool.QueryRow(ctx, query, telegramID, date).Scan(&requestCount, &totalTokens)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get daily usage: %w", err)
	}

	return requestCount, totalTokens, nil
}

// GetUserStats retrieves overall statistics for a user
func (s *Storage) GetUserStats(ctx context.Context, telegramID int64) (totalRequests int64, totalTokens int64, err error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(ul.total_tokens), 0) as total_tokens
		FROM users u
		LEFT JOIN usage_logs ul ON u.id = ul.user_id AND ul.success = TRUE
		WHERE u.telegram_id = $1
		GROUP BY u.id
	`

	err = s.pool.QueryRow(ctx, query, telegramID).Scan(&totalRequests, &totalTokens)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get user stats: %w", err)
	}

	return totalRequests, totalTokens, nil
}

// CleanupOldLogs removes logs older than 90 days
func (s *Storage) CleanupOldLogs(ctx context.Context) (int, error) {
	var deletedCount int

	err := s.pool.QueryRow(ctx, "SELECT cleanup_old_logs()").Scan(&deletedCount)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old logs: %w", err)
	}

	log.Printf("Cleaned up %d old usage logs", deletedCount)
	return deletedCount, nil
}

// TruncateString safely truncates a string to maxLength
func TruncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}

// RemainingTokens returns how many tokens are left in the subscription period
func (s *Subscription) RemainingTokens() int {
	return s.TokensGranted - s.TokensUsed
}

// UpsertSubscription creates or renews a monthly subscription for a user
func (s *Storage) UpsertSubscription(ctx context.Context, userID int64, tokensGranted int, duration time.Duration) (*Subscription, error) {
	sub := &Subscription{}

	query := `
                INSERT INTO subscriptions (user_id, expires_at, tokens_granted, tokens_used)
                VALUES ($1, CURRENT_TIMESTAMP + make_interval(secs => $2), $3, 0)
                ON CONFLICT (user_id) DO UPDATE
                SET expires_at = EXCLUDED.expires_at,
                    tokens_granted = EXCLUDED.tokens_granted,
                    tokens_used = 0,
                    updated_at = CURRENT_TIMESTAMP
                RETURNING id, user_id, expires_at, tokens_granted, tokens_used, created_at, updated_at
        `

	err := s.pool.QueryRow(ctx, query, userID, int64(duration.Seconds()), tokensGranted).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.ExpiresAt,
		&sub.TokensGranted,
		&sub.TokensUsed,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert subscription: %w", err)
	}

	return sub, nil
}

// GetActiveSubscription returns an active subscription for the user if it exists
func (s *Storage) GetActiveSubscription(ctx context.Context, userID int64) (*Subscription, error) {
	sub := &Subscription{}
	query := `
                SELECT id, user_id, expires_at, tokens_granted, tokens_used, created_at, updated_at
                FROM subscriptions
                WHERE user_id = $1 AND expires_at > CURRENT_TIMESTAMP
        `

	err := s.pool.QueryRow(ctx, query, userID).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.ExpiresAt,
		&sub.TokensGranted,
		&sub.TokensUsed,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if err == pgxpool.ErrClosed {
			return nil, fmt.Errorf("database connection closed: %w", err)
		}
		return nil, err
	}

	return sub, nil
}

// ConsumeSubscriptionTokens deducts tokens from an active subscription if enough balance exists
func (s *Storage) ConsumeSubscriptionTokens(ctx context.Context, userID int64, tokens int) (*Subscription, bool, error) {
	sub := &Subscription{}

	query := `
                UPDATE subscriptions
                SET tokens_used = tokens_used + $1, updated_at = CURRENT_TIMESTAMP
                WHERE user_id = $2
                  AND expires_at > CURRENT_TIMESTAMP
                  AND tokens_used + $1 <= tokens_granted
                RETURNING id, user_id, expires_at, tokens_granted, tokens_used, created_at, updated_at
        `

	err := s.pool.QueryRow(ctx, query, tokens, userID).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.ExpiresAt,
		&sub.TokensGranted,
		&sub.TokensUsed,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}

	return sub, true, nil
}
