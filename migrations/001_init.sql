-- Corporate Bullshifter Database Schema

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT UNIQUE NOT NULL,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_active TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_telegram_id ON users(telegram_id);
CREATE INDEX idx_users_last_active ON users(last_active);

-- Usage logs table
CREATE TABLE IF NOT EXISTS usage_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    message_preview TEXT,
    response_preview TEXT,
    model VARCHAR(100),
    success BOOLEAN DEFAULT TRUE
);

CREATE INDEX idx_usage_logs_user_id ON usage_logs(user_id);
CREATE INDEX idx_usage_logs_timestamp ON usage_logs(timestamp);
CREATE INDEX idx_usage_logs_user_timestamp ON usage_logs(user_id, timestamp);

-- Daily usage summary view (for analytics)
CREATE OR REPLACE VIEW daily_usage_summary AS
SELECT
    user_id,
    DATE(timestamp) as usage_date,
    COUNT(*) as request_count,
    SUM(total_tokens) as total_tokens,
    SUM(input_tokens) as total_input_tokens,
    SUM(output_tokens) as total_output_tokens
FROM usage_logs
WHERE success = TRUE
GROUP BY user_id, DATE(timestamp);

-- Function to get user's daily usage
CREATE OR REPLACE FUNCTION get_user_daily_usage(
    p_telegram_id BIGINT,
    p_date DATE DEFAULT CURRENT_DATE
)
RETURNS TABLE (
    request_count BIGINT,
    total_tokens BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        COUNT(*)::BIGINT as request_count,
        COALESCE(SUM(ul.total_tokens), 0)::BIGINT as total_tokens
    FROM users u
    LEFT JOIN usage_logs ul ON u.id = ul.user_id
        AND DATE(ul.timestamp) = p_date
        AND ul.success = TRUE
    WHERE u.telegram_id = p_telegram_id
    GROUP BY u.id;
END;
$$ LANGUAGE plpgsql;

-- Function to cleanup old logs (keep last 90 days)
CREATE OR REPLACE FUNCTION cleanup_old_logs()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM usage_logs
    WHERE timestamp < CURRENT_TIMESTAMP - INTERVAL '90 days';

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Comments for documentation
COMMENT ON TABLE users IS 'Telegram users who have interacted with the bot';
COMMENT ON TABLE usage_logs IS 'Log of all API requests with token usage';
COMMENT ON COLUMN usage_logs.message_preview IS 'First 500 chars of user message';
COMMENT ON COLUMN usage_logs.response_preview IS 'First 500 chars of bot response';
COMMENT ON VIEW daily_usage_summary IS 'Aggregated daily usage statistics per user';
