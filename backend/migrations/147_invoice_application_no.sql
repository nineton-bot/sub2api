-- 147_invoice_application_no.sql
-- 新增 invoices.application_no（内部申请单号）。
--
-- 背景：列表里只显示 invoice_no 时，未开具的 pending / approved / rejected 行没有号码，
-- UI 不得不退化展示 "#{id}"，与已开具行的 "26051502000020095606" 形态不一致，体验混乱。
-- 加一列稳定的「申请单号」，全状态都有，与平台真实发票号 invoice_no 分开展示。
--
-- 格式：APP-YYYYMMDD-{id 6 位补零}，例如 APP-20260515-000014
--
-- 旧数据回填用 submitted_at 推日期；新数据由 service 层在 CreateApplication 后回写。

ALTER TABLE invoices ADD COLUMN application_no varchar(32) NOT NULL DEFAULT '';

UPDATE invoices
SET application_no = 'APP-' || to_char(submitted_at, 'YYYYMMDD') || '-' || lpad(id::text, 6, '0')
WHERE application_no = '';

CREATE UNIQUE INDEX invoice_application_no_unique
  ON invoices (application_no)
  WHERE application_no <> '';
