-- 144_invoice_provider_v3.sql
-- v3 自动开票 + 自动红冲对接（财云通 / bigfintax）。
--
-- 在 142_invoices.sql 的基础上扩展：
--   * invoices 新增 12 个字段，承载票种 / Provider 子状态机 / 红冲多步状态机
--   * 新增唯一条件索引：provider_trace_id、reverse_trace_id（仅在非空时唯一）
--   * 新增 refund_requests 表，承接「已开票订单的用户退款 → 自动红冲」全流程
--
-- 设计细节见 invoice_service.go 的 InvoiceStatus / ProviderState / ReverseStep 常量。

-- =============================================================================
-- 1) invoices 扩展
-- =============================================================================
ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS invoice_kind          VARCHAR(10)  NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS invoice_type_code     VARCHAR(4)   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_state        VARCHAR(20)  NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS provider_trace_id     VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_last_error   TEXT         NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_retry_count  INTEGER      NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reverse_step          VARCHAR(20)  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS red_advice_num        VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS red_confirm_num       VARCHAR(60)  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reverse_trace_id      VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS red_invoice_no        VARCHAR(64)  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS red_pdf_path          VARCHAR(512) NOT NULL DEFAULT '';

-- provider_state 高频查询入口（worker 每轮按状态扫描）
CREATE INDEX IF NOT EXISTS idx_invoices_provider_state ON invoices (provider_state);

-- 蓝票 / 红票 trace_id 唯一（空值不约束，便于历史数据 / manual 渠道共存）
CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_provider_trace_id_unique
    ON invoices (provider_trace_id) WHERE provider_trace_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_reverse_trace_id_unique
    ON invoices (reverse_trace_id) WHERE reverse_trace_id <> '';

-- =============================================================================
-- 2) refund_requests
-- =============================================================================
CREATE TABLE IF NOT EXISTS refund_requests (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    payment_order_id VARCHAR(64) NOT NULL,
    invoice_id BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'awaiting_reverse',
    reason TEXT NOT NULL DEFAULT '',
    amount DECIMAL(20,2) NOT NULL,
    admin_id BIGINT,
    admin_notes TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refund_requests_user_status
    ON refund_requests (user_id, status);
CREATE INDEX IF NOT EXISTS idx_refund_requests_order
    ON refund_requests (payment_order_id);
CREATE INDEX IF NOT EXISTS idx_refund_requests_invoice
    ON refund_requests (invoice_id);
CREATE INDEX IF NOT EXISTS idx_refund_requests_status_created
    ON refund_requests (status, created_at);
