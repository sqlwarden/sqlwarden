const INTERNAL_ORIGIN = 'http://sqlwarden.internal'

export type LoginSearch = { redirect?: string }

/** Accepts only same-origin paths and normalizes them for post-login navigation. */
export function safeInternalRedirect(value: unknown): string | undefined {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//'))
    return undefined
  // Browsers treat backslashes as path separators in special URLs, which can
  // turn a value such as /\example.com into a scheme-relative URL.
  if (value.includes('\\')) return undefined

  try {
    const url = new URL(value, INTERNAL_ORIGIN)
    if (url.origin !== INTERNAL_ORIGIN) return undefined
    return `${url.pathname}${url.search}${url.hash}`
  } catch {
    return undefined
  }
}

export function parseLoginSearch(search: Record<string, unknown>): LoginSearch {
  const redirect = safeInternalRedirect(search.redirect)
  return redirect ? { redirect } : {}
}

/** Builds login search state without creating a redirect loop from /login. */
export function loginSearchFor(currentHref: string): LoginSearch {
  const redirect = safeInternalRedirect(currentHref)
  if (!redirect) return {}

  const pathname = new URL(redirect, INTERNAL_ORIGIN).pathname
  return pathname === '/login' || pathname === '/login/' ? {} : { redirect }
}
