/**
 * Admin Referral (邀请返佣) API endpoints
 */

import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'

export interface GlobalReferralStats {
  total_invited_users: number
  total_released: number
  total_pending: number
  total_gross_commission: number
  referee_bonus_granted: number
  referee_bonus_pending: number
  commission_rate: number
  referee_bonus_amount: number
  enabled: boolean
}

export interface ReferrerRank {
  user_id: number
  email: string
  username: string
  invited_count: number
  gross_commission: number
  released_commission: number
}

export interface RefereeDrilldown {
  referee_id: number
  email: string
  username: string
  joined_at: string
  gross_commission: number
  released_commission: number
  order_count: number
  bonus_granted: boolean
}

/** GET /admin/referral/overview */
export async function getOverview(): Promise<GlobalReferralStats> {
  const { data } = await apiClient.get<GlobalReferralStats>('/admin/referral/overview')
  return data
}

/** GET /admin/referral/top?sort=commission|count&limit= */
export async function listTopReferrers(
  sort: 'commission' | 'count' = 'commission',
  limit = 20
): Promise<ReferrerRank[]> {
  const { data } = await apiClient.get<ReferrerRank[]>('/admin/referral/top', {
    params: { sort, limit }
  })
  return data
}

/** GET /admin/referral/user/:id?page=&page_size= */
export async function getReferrerDrilldown(
  referrerId: number,
  page = 1,
  pageSize = 20
): Promise<BasePaginationResponse<RefereeDrilldown>> {
  const { data } = await apiClient.get<BasePaginationResponse<RefereeDrilldown>>(
    `/admin/referral/user/${referrerId}`,
    { params: { page, page_size: pageSize } }
  )
  return data
}

const referralAPI = {
  getOverview,
  listTopReferrers,
  getReferrerDrilldown
}

export default referralAPI
