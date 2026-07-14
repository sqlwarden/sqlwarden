import { describe, expect, it } from 'vitest'
import { formatDate } from './format'
import { trimTrailingSlash } from './utils'

describe('formatDate', () => {
  it('formats valid dates and rejects absent or invalid values', () => {
    expect(formatDate('not-a-date')).toBe('Unknown')
    expect(formatDate()).toBe('Unknown')
    expect(formatDate('2026-01-02T00:00:00Z')).not.toBe('Unknown')
  })
})

describe('trimTrailingSlash', () => {
  it('removes one trailing slash without changing the root path', () => {
    expect(trimTrailingSlash('/orgs/acme/')).toBe('/orgs/acme')
    expect(trimTrailingSlash('/orgs/acme')).toBe('/orgs/acme')
    expect(trimTrailingSlash('/')).toBe('/')
  })
})
