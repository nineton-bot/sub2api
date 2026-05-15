/**
 * Admin Invoice API endpoints
 */

import { apiClient } from '../client'
import { downloadBlobResponse } from '../invoice_download'
import type { BasePaginationResponse } from '@/types'
import type { Invoice, InvoiceDetail } from '@/types/invoice'

export interface AdminInvoiceListParams {
  page?: number
  page_size?: number
  status?: string
  user_id?: number
  email?: string
  void_pending?: boolean
}

export const adminInvoiceAPI = {
  /** 管理员发票列表 */
  list(params?: AdminInvoiceListParams) {
    return apiClient.get<BasePaginationResponse<Invoice>>('/admin/invoices', { params })
  },

  /** 详情 */
  detail(id: number) {
    return apiClient.get<InvoiceDetail>(`/admin/invoices/${id}`)
  },

  /**
   * 通过审核。v3 起携带票种 + 开票渠道；invoice_kind/provider 为空时由服务端使用全局默认。
   * 选择自动开票渠道（如 caiyuntong）会触发后台 worker 异步开具发票。
   */
  approve(id: number, payload?: { notes?: string; invoice_kind?: 'normal' | 'special'; provider?: string }) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/approve`, {
      notes: payload?.notes ?? '',
      invoice_kind: payload?.invoice_kind ?? '',
      provider: payload?.provider ?? '',
    })
  },

  /** 驳回 */
  reject(id: number, reason: string) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/reject`, { reason })
  },

  /** v3：重新尝试自动开票（仅 provider_state=failed）。 */
  retryIssue(id: number) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/retry-issue`, {})
  },

  /** v3：重新尝试自动红冲（仅 provider_state=reverse_failed）。 */
  retryReverse(id: number) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/retry-reverse`, {})
  },

  /** v3：标记「已在第三方平台手工红冲」兜底通道。 */
  markReversed(id: number, redInvoiceNo: string) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/mark-reversed`, { red_invoice_no: redInvoiceNo })
  },

  /** 上传 PDF（multipart） */
  uploadPdf(id: number, file: File, invoiceNo: string, onProgress?: (percent: number) => void) {
    const fd = new FormData()
    fd.append('file', file)
    if (invoiceNo) fd.append('invoice_no', invoiceNo)
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/upload-pdf`, fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (e) => {
        if (e.total && onProgress) {
          onProgress(Math.round((e.loaded / e.total) * 100))
        }
      },
    })
  },

  /** 替换 PDF */
  replacePdf(id: number, file: File, onProgress?: (percent: number) => void) {
    const fd = new FormData()
    fd.append('file', file)
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/replace-pdf`, fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (e) => {
        if (e.total && onProgress) {
          onProgress(Math.round((e.loaded / e.total) * 100))
        }
      },
    })
  },

  /** 标记已开具（无 PDF 通道） */
  markIssued(id: number, invoiceNo: string) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/mark-issued`, { invoice_no: invoiceNo })
  },

  /** 作废 */
  void(id: number, reason: string) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/void`, { reason })
  },

  /** 通过用户作废申请（同事务调用 AdminVoid，触发红冲） */
  approveVoidRequest(requestId: number, notes: string) {
    return apiClient.post<{ message: string }>(
      `/admin/invoice-void-requests/${requestId}/approve`,
      { notes },
    )
  },

  /** 驳回用户作废申请 */
  rejectVoidRequest(requestId: number, reason: string) {
    return apiClient.post<{ message: string }>(
      `/admin/invoice-void-requests/${requestId}/reject`,
      { reason },
    )
  },

  /** 管理员下载 PDF（带 JWT auth） */
  async downloadPdf(id: number): Promise<void> {
    const resp = await apiClient.get(`/admin/invoices/${id}/pdf`, { responseType: 'blob' })
    downloadBlobResponse(resp, `invoice-${id}.pdf`)
  },

  /** 单用户发票可见性 — 读 */
  getUserConfig(userId: number) {
    return apiClient.get<{ enabled: boolean }>(`/admin/users/${userId}/invoice-config`)
  },

  /** 单用户发票可见性 — 写 */
  setUserConfig(userId: number, enabled: boolean) {
    return apiClient.put<{ enabled: boolean }>(`/admin/users/${userId}/invoice-config`, { enabled })
  },
}

export default adminInvoiceAPI
