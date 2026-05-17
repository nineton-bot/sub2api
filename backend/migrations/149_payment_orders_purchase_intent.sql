-- 149_payment_orders_purchase_intent.sql
-- 给 payment_orders 表增加"购买意图"两个字段，区分续期 vs 新购（叠加套餐），
-- 让 payment_fulfillment.doSub 按 intent 路由到 RenewSubscription 或 CreateNewSubscription。
--
-- 背景：148 放开了 (user_id, group_id) 唯一约束后，支付路径默认走 CreateNewSubscription
-- 总是新建一行（叠加套餐）。但"我的订阅"页的"续期"按钮 —— 用户意图明确就是延期，
-- 不应该被改成新建一行。前端弹窗让用户在"续期"和"再买一张"之间二选一，结果通过
-- purchase_intent 字段透传到后端，履约时按 intent 走不同分支。
--
-- 字段：
--   purchase_intent: 'new'（默认） / 'renew'
--   renew_subscription_id: 仅 purchase_intent='renew' 时使用；履约时验证归属
--
-- 兼容性：所有历史行 purchase_intent 默认 'new'，行为与历史完全一致（CreateNewSubscription）。
-- 不加外键到 user_subscriptions：sub 被硬删后订单仍保留可审计性。

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS purchase_intent VARCHAR(16) NOT NULL DEFAULT 'new';

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS renew_subscription_id BIGINT;

-- 部分索引：仅追踪续期订单，加速按 renew_subscription_id 反查（少量行）
CREATE INDEX IF NOT EXISTS payment_orders_renew_subscription_idx
    ON payment_orders(renew_subscription_id)
    WHERE renew_subscription_id IS NOT NULL;
