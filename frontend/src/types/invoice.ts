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

export interface Invoice {
  id: number
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
}
