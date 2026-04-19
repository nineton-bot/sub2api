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
  /**
   * V2 可使用佣金池（users.referral_usable）。
   * 仅包含 V2 改造后新释放、尚未转入余额或提现的部分；历史已释放额度不会回溯进来。
   */
  usable_commission: number
  commission_rate: number
  referee_bonus_amount: number
  /** 是否允许提现（admin 在单用户 override 中设置，默认 false） */
  withdrawal_allowed: boolean
}

export interface EligibilityResponse {
  enabled: boolean
  reason:
    | 'feature_disabled'
    | 'disabled_for_user'
    | 'user_override'
    | 'global_default'
    | 'not_enabled_for_user'
    | string
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

/**
 * GET /user/referral/eligibility — 当前用户是否可见/可用推广页。
 * 前端 ReferralView mount 时调用，enabled=false 时展示占位；
 * 用于侧边栏/路由守卫判断是否显示入口。
 */
export async function getEligibility(): Promise<EligibilityResponse> {
  const { data } = await apiClient.get<EligibilityResponse>('/user/referral/eligibility')
  return data
}

/**
 * POST /user/referral/transfer-to-balance — 从 referral_usable 池转入账户余额。
 *
 * 业务规则：
 * - amount 必须 >= 0.01（ReferralUsableMinTransfer）
 * - referral_usable 必须 >= amount，否则返回 INSUFFICIENT_REFERRAL_USABLE
 * - 成功后后端返回最新 ReferralStats，前端据此刷新卡片
 */
export async function transferToBalance(amount: number): Promise<ReferralStats> {
  const { data } = await apiClient.post<ReferralStats>(
    '/user/referral/transfer-to-balance',
    { amount }
  )
  return data
}

/**
 * 按天聚合的释放日志（用户端展开单笔 commission 时使用）。
 * - day: 当日零点（UTC ISO8601）
 * - trigger_type: recharge_consumed | subscription_daily | manual_admin | refund_reversal
 * - total_amount: 当日该触发类型净释放金额，可为负（退款反转时）
 * - event_count: 当日该触发类型的释放事件数
 */
export interface ReleaseLogDayAggregate {
  day: string
  trigger_type: string
  total_amount: number
  event_count: number
}

/** GET /user/referral/release-logs?commission_id=&page=&size= */
export async function listReleaseLogsDaily(
  commissionId?: number,
  page = 1,
  pageSize = 30
): Promise<BasePaginationResponse<ReleaseLogDayAggregate>> {
  const params: Record<string, string | number> = { page, page_size: pageSize }
  if (commissionId) params.commission_id = commissionId
  const { data } = await apiClient.get<BasePaginationResponse<ReleaseLogDayAggregate>>(
    '/user/referral/release-logs',
    { params }
  )
  return data
}

export const referralAPI = {
  getOverview,
  listCommissions,
  ensureInviteCode,
  getEligibility,
  transferToBalance,
  listReleaseLogsDaily
}

export default referralAPI
