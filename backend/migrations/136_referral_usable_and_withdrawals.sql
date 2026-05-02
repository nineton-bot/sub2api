-- 105_referral_usable_and_withdrawals.sql
-- V2 差异化返佣改造：新增
--   * users.referral_usable：独立的可使用佣金池（与 user.balance 解耦）
--   * user_referral_configs：单用户覆盖（是否启用 / 比例 / 赠金 / 可提现）
--   * referral_withdrawals：提现申请 + 管理员审批生命周期
--   * referral_commission_release_logs：append-only 释放日志，供用户查看计算明细
--   * settings 种子新 key referral_default_for_all_users

-- 1. 可使用佣金池
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS referral_usable DECIMAL(20,8) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_users_referral_usable ON users(referral_usable) WHERE referral_usable > 0;

-- 2. 单用户返佣配置覆盖
CREATE TABLE IF NOT EXISTS user_referral_configs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN,                              -- NULL = 跟随全局默认
    commission_rate_override DECIMAL(10,6),       -- NULL = 跟随全局
    referee_bonus_override DECIMAL(20,8),         -- NULL = 跟随全局
    withdrawal_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_urc_user ON user_referral_configs(user_id);

-- 3. 提现申请
CREATE TABLE IF NOT EXISTS referral_withdrawals (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(20,8) NOT NULL CHECK (amount > 0),
    payout_method VARCHAR(20) NOT NULL,            -- wechat | alipay | bank | other
    payout_account TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | approved | rejected | completed | cancelled
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    review_notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refwd_user_status ON referral_withdrawals(user_id, status);
CREATE INDEX IF NOT EXISTS idx_refwd_status_requested ON referral_withdrawals(status, requested_at DESC);

-- 4. Append-only 释放日志
CREATE TABLE IF NOT EXISTS referral_commission_release_logs (
    id BIGSERIAL PRIMARY KEY,
    commission_id BIGINT NOT NULL REFERENCES referral_commissions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(20,8) NOT NULL,                                      -- 本次释放增量，退款反转可为负
    trigger_type VARCHAR(30) NOT NULL,                                  -- recharge_consumed | subscription_daily | manual_admin | refund_reversal
    rate_snapshot DECIMAL(10,6) NOT NULL,
    computation_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refcomm_log_commission ON referral_commission_release_logs(commission_id);
CREATE INDEX IF NOT EXISTS idx_refcomm_log_user_created ON referral_commission_release_logs(user_id, created_at DESC);

-- 5. 种子新设置（默认保持 V1 行为：全员可见）
-- settings 表只有 key/value/updated_at（无 created_at），不能在此处显式写入 created_at
INSERT INTO settings (key, value, updated_at)
VALUES ('referral_default_for_all_users', 'true', NOW())
ON CONFLICT (key) DO NOTHING;
