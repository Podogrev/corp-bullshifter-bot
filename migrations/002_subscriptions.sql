-- Subscription support for Telegram Stars monetization

CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    tokens_granted INTEGER NOT NULL DEFAULT 0,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_expiry ON subscriptions(expires_at);

COMMENT ON TABLE subscriptions IS 'Recurring month-long token packages purchased with Telegram Stars';
COMMENT ON COLUMN subscriptions.tokens_granted IS 'Total tokens granted for the current period';
COMMENT ON COLUMN subscriptions.tokens_used IS 'Tokens consumed in the current period';
