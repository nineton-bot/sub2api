-- 103_add_referral_system.sql
-- Adds the referral/commission incentive system.
--   * Extends users with an invited_by_user_id back-pointer and a
--     per-user invite_code.
--   * Creates referral_commissions as the commission ledger (one row per
--     recharge or subscription payment made by a referee).
--   * Creates referral_pending_bonuses for the delayed referee welcome bonus
--     that is only granted on the referee's first paid recharge or subscription.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS invited_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS invite_code VARCHAR(16);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_invite_code_unique ON users(invite_code) WHERE invite_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_invited_by ON users(invited_by_user_id);

CREATE TABLE IF NOT EXISTS referral_commissions (
    id BIGSERIAL PRIMARY KEY,
    referrer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    referee_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type VARCHAR(20) NOT NULL,                   -- 'recharge' | 'subscription'
    source_order_id BIGINT NOT NULL,
    source_amount DECIMAL(20,8) NOT NULL,
    source_subscription_id BIGINT,
    source_validity_days INTEGER,
    source_starts_at TIMESTAMPTZ,
    commission_rate DECIMAL(10,6) NOT NULL,
    gross_commission DECIMAL(20,8) NOT NULL,
    released_commission DECIMAL(20,8) NOT NULL DEFAULT 0,
    consumed_attributed DECIMAL(20,8) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'accruing',     -- accruing | fully_released | reversed | partial_reversed
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_refcomm_order_type ON referral_commissions(source_order_id, source_type);
CREATE INDEX IF NOT EXISTS idx_refcomm_referrer ON referral_commissions(referrer_id, status);
CREATE INDEX IF NOT EXISTS idx_refcomm_referee ON referral_commissions(referee_id);
CREATE INDEX IF NOT EXISTS idx_refcomm_source_type ON referral_commissions(source_type, status);

CREATE TABLE IF NOT EXISTS referral_pending_bonuses (
    id BIGSERIAL PRIMARY KEY,
    referee_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    referrer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bonus_amount DECIMAL(20,8) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',      -- pending | granted
    granted_at TIMESTAMPTZ,
    granted_trigger VARCHAR(20),                        -- 'first_recharge' | 'first_subscription'
    granted_order_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refbonus_referee_status ON referral_pending_bonuses(referee_id, status);
CREATE INDEX IF NOT EXISTS idx_refbonus_referrer ON referral_pending_bonuses(referrer_id);
