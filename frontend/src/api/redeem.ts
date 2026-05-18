/**
 * Redeem code API endpoints
 * Handles redeem code redemption for users
 */

import { apiClient } from './client'
import type { RedeemCodeRequest } from '@/types'

export interface RedeemHistoryItem {
  id: number
  code: string
  type: string
  value: number
  status: string
  used_at: string
  created_at: string
  // Notes from admin for admin_balance/admin_concurrency types
  notes?: string
  // Subscription-specific fields
  group_id?: number
  validity_days?: number
  group?: {
    id: number
    name: string
  }
}

/**
 * Preview response — describes what the code does without consuming it.
 * subscription 类型才会带 group / existing_active_subs 字段。
 */
export interface RedeemPreviewSubInfo {
  id: number
  expires_at: string
}

export interface RedeemPreviewResponse {
  type: 'balance' | 'concurrency' | 'subscription' | 'invitation' | string
  value: number
  group_id?: number
  group_name?: string
  validity_days?: number
  existing_active_subs?: RedeemPreviewSubInfo[]
  stack_cap?: number
  stack_count?: number
  is_reduce?: boolean
}

/**
 * Redeem a code
 * @param code - Redeem code string
 * @param purchaseIntent - subscription 类型可选："renew" 续期 / "new" 再买一张；缺省走老行为
 * @param renewSubscriptionId - purchaseIntent="renew" 时必填，目标订阅 ID
 * @returns Redemption result with updated balance or concurrency
 */
export async function redeem(
  code: string,
  purchaseIntent?: 'renew' | 'new',
  renewSubscriptionId?: number,
): Promise<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
}> {
  const payload: RedeemCodeRequest = { code }
  if (purchaseIntent) {
    payload.purchase_intent = purchaseIntent
  }
  if (purchaseIntent === 'renew' && renewSubscriptionId) {
    payload.renew_subscription_id = renewSubscriptionId
  }

  const { data } = await apiClient.post<{
    message: string
    type: string
    value: number
    new_balance?: number
    new_concurrency?: number
  }>('/redeem', payload)

  return data
}

/**
 * Preview a redeem code without consuming it (read-only).
 * 用于兑换前判断是否需要弹"续期 vs 再买一张"二选一弹窗。
 */
export async function preview(code: string): Promise<RedeemPreviewResponse> {
  const { data } = await apiClient.post<RedeemPreviewResponse>('/redeem/preview', { code })
  return data
}

/**
 * Get user's redemption history
 * @returns List of redeemed codes
 */
export async function getHistory(): Promise<RedeemHistoryItem[]> {
  const { data } = await apiClient.get<RedeemHistoryItem[]>('/redeem/history')
  return data
}

export const redeemAPI = {
  redeem,
  preview,
  getHistory
}

export default redeemAPI
