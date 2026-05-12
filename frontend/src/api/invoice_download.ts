import type { AxiosResponse } from 'axios'

function parseFilename(disposition: string | undefined): string | null {
  if (!disposition) return null
  const utf8 = disposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8 && utf8[1]) {
    try {
      return decodeURIComponent(utf8[1])
    } catch {
      // fall through
    }
  }
  const ascii = disposition.match(/filename="?([^";]+)"?/i)
  return ascii ? ascii[1] : null
}

export function downloadBlobResponse(resp: AxiosResponse<Blob>, fallbackName: string): void {
  const blob = resp.data
  const cd = (resp.headers && (resp.headers['content-disposition'] || resp.headers['Content-Disposition'])) as
    | string
    | undefined
  const filename = parseFilename(cd) || fallbackName
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  window.URL.revokeObjectURL(url)
}
