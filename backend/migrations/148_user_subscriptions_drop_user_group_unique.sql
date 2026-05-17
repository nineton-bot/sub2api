-- 148_user_subscriptions_drop_user_group_unique.sql
-- 放开 user_subscriptions 表 (user_id, group_id) 的唯一约束，允许同一用户对同一分组
-- 拥有多张并行订阅（"叠加套餐"）。
--
-- 背景：原 partial unique index `user_subscriptions_user_group_unique_active`
-- (migrations/016) 强制每个用户对每个分组最多一条 active 记录。这使得支付路径下
-- 重复购买同一套餐时只能走"延长 expires_at"分支（subscription_service.go
-- AssignOrExtendSubscription），但 monthly_usage_usd / monthly_window_start 不会
-- 重置 —— 用户在配额已耗尽的窗口里再次付费却无法立即使用，体感像"白买"。
--
-- 改造后：
--   - 支付履约路径下，同 (user_id, group_id) 的二次购买会新插入一行，配额独立、
--     窗口从购买时刻起算
--   - 请求计费层按 expires_at ASC 排序挑第一张未触限的 sub（FIFO + 跳过耗尽）
--   - 兑换码 / admin 赋予路径仍走原 AssignOrExtendSubscription 续期语义
--
-- 单向迁移：回滚前需先去重 active 行，否则重建 unique index 会失败。

ALTER TABLE user_subscriptions
    DROP CONSTRAINT IF EXISTS user_subscriptions_user_id_group_id_key;

DROP INDEX IF EXISTS user_subscriptions_user_group_unique_active;

-- 保留 (user_id, group_id) 普通索引以加速 ListActiveByUserIDAndGroupID 等查询
CREATE INDEX IF NOT EXISTS user_subscriptions_user_group_idx
    ON user_subscriptions(user_id, group_id)
    WHERE deleted_at IS NULL;
