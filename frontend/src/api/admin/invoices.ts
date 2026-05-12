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

  /** 通过审核 */
  approve(id: number, notes?: string) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/approve`, { notes: notes ?? '' })
  },

  /** 驳回 */
  reject(id: number, reason: string) {
    return apiClient.post<{ message: string }>(`/admin/invoices/${id}/reject`, { reason })
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
