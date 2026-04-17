/**
 * Referral (邀请返佣) API endpoints — user side
 */

import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'

export interface ReferralStats {
  invite_code: string
  invited_count: number
  gross_commission: number
  released_commission: number
  pending_commission: number
  commission_rate: number
  referee_bonus_amount: number
}

export interface CommissionLog {
  id: number
  referee_id: number
  referee_email: string
  source_type: 'recharge' | 'subscription' | string
  source_order_id: number
  source_amount: number
  commission_rate: number
  gross_commission: number
  released_commission: number
  status: 'accruing' | 'fully_released' | 'reversed' | 'partial_reversed' | string
  source_validity_days?: number | null
  created_at: string
  updated_at: string
}

export interface EnsureInviteCodeResponse {
  invite_code: string
  enabled: boolean
}

/** GET /user/referral/overview */
export async function getOverview(): Promise<ReferralStats> {
  const { data } = await apiClient.get<ReferralStats>('/user/referral/overview')
  return data
}

/** GET /user/referral/commissions?page=&size= */
export async function listCommissions(
  page = 1,
  pageSize = 20
): Promise<BasePaginationResponse<CommissionLog>> {
  const { data } = await apiClient.get<BasePaginationResponse<CommissionLog>>(
    '/user/referral/commissions',
    { params: { page, page_size: pageSize } }
  )
  return data
}

/** POST /user/referral/ensure-code — 幂等生成/返回邀请码 */
export async function ensureInviteCode(): Promise<EnsureInviteCodeResponse> {
  const { data } = await apiClient.post<EnsureInviteCodeResponse>(
    '/user/referral/ensure-code'
  )
  return data
}

export const referralAPI = {
  getOverview,
  listCommissions,
  ensureInviteCode
}

export default referralAPI
