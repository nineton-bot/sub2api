/**
 * User Invoice API endpoints
 */

import { apiClient } from './client'
import { downloadBlobResponse } from './invoice_download'
import type { BasePaginationResponse } from '@/types'
import type {
  EligibleOrder,
  Invoice,
  InvoiceDetail,
  CreateInvoiceRequest,
  LastInvoiceTitle,
} from '@/types/invoice'

export const invoiceAPI = {
  /** 当前用户是否对发票功能可见（侧栏菜单 featureFlag 用） */
  eligibility() {
    return apiClient.get<{ enabled: boolean }>('/invoices/eligibility')
  },

  /** 列出半年内可开票订单，附带上次申请抬头与最低开票金额 */
  eligibleOrders() {
    return apiClient.get<{
      items: EligibleOrder[]
      last_title: LastInvoiceTitle | null
      min_amount: number
    }>('/invoices/eligible-orders')
  },

  /** 当前用户发票列表 */
  list(params?: { page?: number; page_size?: number; status?: string }) {
    return apiClient.get<BasePaginationResponse<Invoice>>('/invoices', { params })
  },

  /** 提交发票申请 */
  create(data: CreateInvoiceRequest) {
    return apiClient.post<InvoiceDetail>('/invoices', data)
  },

  /** 单条详情 */
  detail(id: number) {
    return apiClient.get<InvoiceDetail>(`/invoices/${id}`)
  },

  /** 用户取消（仅 pending） */
  cancel(id: number) {
    return apiClient.post<{ message: string }>(`/invoices/${id}/cancel`)
  },

  /** 申请作废（仅 issued + 非 manual 渠道） */
  requestVoid(id: number, reason: string) {
    return apiClient.post<{ id: number; status: string; reason: string; requested_at: string }>(
      `/invoices/${id}/void-request`,
      { reason },
    )
  },

  /** 已开具发票 PDF 下载（带 JWT auth，触发浏览器下载） */
  async downloadPdf(id: number): Promise<void> {
    const resp = await apiClient.get(`/invoices/${id}/pdf`, { responseType: 'blob' })
    downloadBlobResponse(resp, `invoice-${id}.pdf`)
  },
}

export default invoiceAPI
