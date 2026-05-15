// Stub: affiliate 整套已永久排除（详见 sync_upstream_2026_05.md）。
// 保留 no-op 函数避免 OAuth 回调视图 import 报错；所有函数返回空值/空操作。

export function resolveAffiliateReferralCode(..._args: unknown[]): string {
  return ''
}

export function storeOAuthAffiliateCode(..._args: unknown[]): void {
  // no-op
}

export function clearAllAffiliateReferralCodes(..._args: unknown[]): void {
  // no-op
}

export function loadOAuthAffiliateCode(..._args: unknown[]): string {
  return ''
}

export function oauthAffiliatePayload(..._args: unknown[]): Record<string, never> {
  return {}
}

export function clearAffiliateReferralCode(..._args: unknown[]): void {
  // no-op
}

export function loadAffiliateReferralCode(..._args: unknown[]): string {
  return ''
}
