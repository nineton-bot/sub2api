-- 150_invoice_bank_transfer.sql
-- 对公转账发票：用户通过银行对公转账付款，系统内没有对应订单，
-- 申请发票时手填转账金额与日期。管理员需先「确认收款」才能审批开票。
--
-- 新字段对订单开票发票无影响：source 默认 'order'，其余默认空/false。

ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS source                VARCHAR(20) NOT NULL DEFAULT 'order',
    ADD COLUMN IF NOT EXISTS transfer_date         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS transfer_confirmed    BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS transfer_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS transfer_confirmed_by BIGINT;
