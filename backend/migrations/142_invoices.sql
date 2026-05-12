-- 142_invoices.sql
-- Adds the invoice system:
--   * invoices            — 用户开票申请，5 态状态机（pending/approved/issued/rejected/voided）
--   * invoice_items       — 发票与 payment_orders 的 1:N 关联，UNIQUE(payment_order_id) 保证一个订单
--                           最多被一张活跃发票占用；service 在 reject/void 时 hard-delete 释放约束
--   * payment_orders 增加 invoice_status / invoice_id 反规范化字段，加速列表展示
--
-- 已开发票订单不允许退款；refund 入口由 service 校验 invoice_status，详见 invoice_service.go。

CREATE TABLE IF NOT EXISTS invoices (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_email VARCHAR(255) NOT NULL,
    title_type VARCHAR(20) NOT NULL,
    title VARCHAR(200) NOT NULL,
    tax_no VARCHAR(64) NOT NULL DEFAULT '',
    contact_email VARCHAR(255) NOT NULL DEFAULT '',
    amount DECIMAL(20,2) NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    notes TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    submitted_at TIMESTAMPTZ NOT NULL,
    reviewed_at TIMESTAMPTZ,
    reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    review_notes TEXT NOT NULL DEFAULT '',
    issued_at TIMESTAMPTZ,
    invoice_no VARCHAR(64) NOT NULL DEFAULT '',
    pdf_path VARCHAR(512) NOT NULL DEFAULT '',
    pdf_storage VARCHAR(20) NOT NULL DEFAULT 'local',
    pdf_size BIGINT NOT NULL DEFAULT 0,
    pdf_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    pdf_original_name VARCHAR(255) NOT NULL DEFAULT '',
    provider VARCHAR(30) NOT NULL DEFAULT 'manual',
    provider_payload JSONB,
    voided_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoices_user_status ON invoices(user_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_status_submitted ON invoices(status, submitted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_invoice_no_unique
    ON invoices(invoice_no) WHERE invoice_no <> '';

CREATE TABLE IF NOT EXISTS invoice_items (
    id BIGSERIAL PRIMARY KEY,
    invoice_id BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
    order_no VARCHAR(64) NOT NULL,
    product_name VARCHAR(200) NOT NULL DEFAULT '',
    order_type VARCHAR(20) NOT NULL,
    pay_amount DECIMAL(20,2) NOT NULL,
    paid_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id);

-- 关键：防重复约束。同一 payment_order 在该表只能存在一行；
-- service 在 reject/void 时 hard-delete 对应行以释放占用，rejected/voided 的 invoice 行
-- 本身保留作为历史记录（其 items 被清空）。
CREATE UNIQUE INDEX IF NOT EXISTS idx_invoice_items_active_order
    ON invoice_items(payment_order_id);

-- payment_orders 反规范化字段
ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS invoice_status VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS invoice_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_payment_orders_invoice_status
    ON payment_orders(invoice_status) WHERE invoice_status <> '';
