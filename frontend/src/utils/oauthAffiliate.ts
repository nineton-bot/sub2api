// No-op shim for upstream's affiliate referral utility.
//
// 我们 fork 不引入 upstream affiliate 系统（保留自家 V2 referral）。upstream merge
// 把 OAuth 回调视图（LinuxDoCallbackView / OidcCallbackView / WechatCallbackView /
// LoginView）和 WechatOAuthSection 改成调用这些工具函数，但函数本体已永久排除。
//
// 这里提供 0 副作用的桩，让前端编译 + 运行不抛错；V2 referrer_code 走另一套
// `referralCookie.ts` + `referrer_code` 字段，与 affiliate 通路完全解耦。
//
// 删除这些桩需要先把上述 5 个调用点全部清掉。

export function normalizeOAuthAffiliateCode(_value?: unknown): string {
  return ''
}

export function pickOAuthAffiliateCode(..._values: unknown[]): string {
  return ''
}

export function storeAffiliateReferralCode(_value?: unknown, _now: number = Date.now()): void {
  // intentional no-op
}

export function loadAffiliateReferralCode(_now: number = Date.now()): string {
  return ''
}

export function clearAffiliateReferralCode(): void {
  // intentional no-op
}

export function resolveAffiliateReferralCode(..._values: unknown[]): string {
  return ''
}

export function storeOAuthAffiliateCode(_value?: unknown): void {
  // intentional no-op
}

export function loadOAuthAffiliateCode(): string {
  return ''
}

export function clearOAuthAffiliateCode(): void {
  // intentional no-op
}

export function clearAllAffiliateReferralCodes(): void {
  // intentional no-op
}

export function oauthAffiliatePayload(_value?: unknown): Record<string, never> {
  return {}
}
