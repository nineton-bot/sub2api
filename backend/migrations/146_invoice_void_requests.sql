-- 146_invoice_void_requests.sql
-- 新增「发票作废申请」工单表。
--
-- 业务场景：用户对已开（issued）发票发起作废请求（例如抬头开错想重开），
-- admin 在发票列表里看到对应行有「待审批作废」徽标，通过/驳回。
-- 通过后服务端同事务调用 AdminVoid，复用现有红冲流水线，自动作废原票。
--
-- 工单本身只跟踪 pending_review → approved / rejected 三态；
-- 红冲后续进度（reverse_pending → reverse_success / reverse_failed）由
-- invoices.provider_state 体现，不在 invoice_void_requests 上重复维护。

CREATE TABLE IF NOT EXISTS invoice_void_requests (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    invoice_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending_review',
    reason TEXT NOT NULL DEFAULT '',
    admin_id BIGINT,
    admin_notes TEXT NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoice_void_requests_user_status
    ON invoice_void_requests (user_id, status);
CREATE INDEX IF NOT EXISTS idx_invoice_void_requests_invoice_status
    ON invoice_void_requests (invoice_id, status);
CREATE INDEX IF NOT EXISTS idx_invoice_void_requests_status_created
    ON invoice_void_requests (status, created_at);
