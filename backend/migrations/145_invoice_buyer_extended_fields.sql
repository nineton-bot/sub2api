-- 145_invoice_buyer_extended_fields.sql
-- v3 后续：开具专票（数电专 05 / 增专 01 / 增专电子 08）时财云通强制要求
-- 购方地址 / 电话 / 开户行 / 银行账号。之前 invoices 表只存 title/tax_no/contact_email，
-- 缺这 4 个字段会被平台直接拒（returns "请求参数异常：发票号码 invoiceNumber 为空"）。
--
-- 新字段对普票/个人发票完全无影响：默认空字符串，service 层在专票时校验非空。

ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS buyer_address      VARCHAR(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS buyer_phone        VARCHAR(32)  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS buyer_bank_name    VARCHAR(64)  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS buyer_bank_account VARCHAR(64)  NOT NULL DEFAULT '';
