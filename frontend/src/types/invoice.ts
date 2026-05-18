/**
 * Invoice (发票) Type Definitions
 */

export type InvoiceStatus = 'pending' | 'approved' | 'issued' | 'rejected' | 'voided'
export type InvoiceTitleType = 'personal' | 'business'

export interface EligibleOrder {
  id: number
  order_no: string
  product_name: string
  order_type: 'balance' | 'subscription'
  pay_amount: number
  paid_at: string
}

/** 用户最近一次发票申请的抬头信息，用于申请表单回显。 */
export interface LastInvoiceTitle {
  title_type: InvoiceTitleType
  title: string
  tax_no: string
  contact_email: string
  buyer_address: string
  buyer_phone: string
  buyer_bank_name: string
  buyer_bank_account: string
}

/** v3：自动开票 / 自动红冲子状态。 */
export type ProviderState =
  | ''
  | 'none'
  | 'queued'
  | 'issuing'
  | 'success'
  | 'failed'
  | 'reverse_pending'
  | 'reversing'
  | 'reverse_success'
  | 'reverse_failed'

export interface Invoice {
  id: number
  /** 内部申请单号，创建即有，格式 APP-YYYYMMDD-000014 */
  application_no: string
  invoice_no: string
  user_id: number
  user_email: string
  title_type: InvoiceTitleType
  title: string
  tax_no: string
  amount: number
  currency: string
  status: InvoiceStatus
  order_count: number
  submitted_at: string
  contact_email: string
  // v3：开票渠道与子状态（manual 渠道时 provider_state 通常为 'none' 或空）
  provider?: string
  provider_state?: ProviderState
  invoice_kind?: 'normal' | 'special'
  provider_last_error?: string

  // 购方扩展信息（专票必填，普票可空）
  buyer_address?: string
  buyer_phone?: string
  buyer_bank_name?: string
  buyer_bank_account?: string

  // 对公转账（source=bank_transfer 时有效）
  source?: 'order' | 'bank_transfer'
  transfer_date?: string | null
  transfer_confirmed?: boolean
  transfer_confirmed_at?: string | null

  /** 用户对该发票提交的作废申请（仅当存在 pending_review 时由后端 inline）。 */
  pending_void_request?: PendingVoidRequestInfo
}

export interface PendingVoidRequestInfo {
  id: number
  reason: string
  requested_at: string
}

export interface InvoiceItem {
  order_id: number
  order_no: string
  product_name: string
  order_type: 'balance' | 'subscription'
  pay_amount: number
  paid_at: string
}

export interface InvoiceDetail extends Invoice {
  notes: string
  reviewed_at?: string | null
  reviewed_by?: number | null
  review_notes: string
  issued_at?: string | null
  pdf_original_name?: string
  pdf_available: boolean
  provider: string
  items: InvoiceItem[]
}

export interface CreateInvoiceRequest {
  title_type: InvoiceTitleType
  title: string
  tax_no: string
  contact_email: string
  notes: string
  order_ids: number[]
  /**
   * 购方扩展信息（开专票必填，开普票可空）。
   * service 端在审批为专票时强制校验非空。
   */
  buyer_address?: string
  buyer_phone?: string
  buyer_bank_name?: string
  buyer_bank_account?: string

  // 对公转账：source=bank_transfer 时 order_ids 为空，改用以下字段
  source?: 'order' | 'bank_transfer'
  transfer_amount?: number
  transfer_date?: string
}
