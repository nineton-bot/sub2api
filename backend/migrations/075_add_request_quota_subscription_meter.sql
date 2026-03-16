ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS subscription_meter VARCHAR(20) NOT NULL DEFAULT 'cost_quota',
    ADD COLUMN IF NOT EXISTS daily_request_limit INTEGER,
    ADD COLUMN IF NOT EXISTS weekly_request_limit INTEGER,
    ADD COLUMN IF NOT EXISTS monthly_request_limit INTEGER;

UPDATE groups
SET subscription_meter = 'cost_quota'
WHERE subscription_meter IS NULL OR subscription_meter = '';

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_request_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_request_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_request_count INTEGER NOT NULL DEFAULT 0;
