// referralCookie.ts — helpers for the invite shortlink ("stealth mode") cookies.
//
// When a visitor clicks a shortlink like /g/<code>, the backend sets two
// cookies (referral_code + referral_stealth=1) then redirects to /register.
// The register page and OAuth entry points read these cookies here so they
// can carry the referrer code through normal OR OAuth registration without
// exposing `?ref=` in the URL.

const REFERRAL_CODE_COOKIE = 'referral_code'
const REFERRAL_STEALTH_COOKIE = 'referral_stealth'

function readCookie(name: string): string {
  if (typeof document === 'undefined') return ''
  const prefix = name + '='
  const parts = document.cookie.split(';')
  for (const raw of parts) {
    const c = raw.trim()
    if (c.startsWith(prefix)) {
      try {
        return decodeURIComponent(c.slice(prefix.length))
      } catch {
        return c.slice(prefix.length)
      }
    }
  }
  return ''
}

export function getReferralCodeFromCookie(): string {
  return readCookie(REFERRAL_CODE_COOKIE)
}

export function isStealthMode(): boolean {
  return readCookie(REFERRAL_STEALTH_COOKIE) === '1'
}

export function clearReferralCookies(): void {
  if (typeof document === 'undefined') return
  document.cookie = `${REFERRAL_CODE_COOKIE}=; Path=/; Max-Age=0`
  document.cookie = `${REFERRAL_STEALTH_COOKIE}=; Path=/; Max-Age=0`
}
