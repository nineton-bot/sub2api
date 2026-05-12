-- 143_user_invoice_enabled.sql
-- Per-user invoice visibility override.
--
-- 与全局开关 invoice_enabled / invoice_default_for_all_users 配合使用：
--   global enabled=false → 全站不可见（此字段忽略）
--   global enabled=true && default_for_all=true  → 全员可见（此字段忽略）
--   global enabled=true && default_for_all=false → 仅 users.invoice_enabled=true 的用户可见

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS invoice_enabled BOOLEAN NOT NULL DEFAULT FALSE;
