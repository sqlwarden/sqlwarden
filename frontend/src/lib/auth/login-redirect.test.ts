import { describe, expect, it } from 'vitest'
import { loginSearchFor, parseLoginSearch, safeInternalRedirect } from './login-redirect'

describe('login redirects', () => {
  it('preserves an internal path with its query and hash', () => {
    expect(safeInternalRedirect('/orgs/acme/users?q=alex#details')).toBe(
      '/orgs/acme/users?q=alex#details',
    )
    expect(parseLoginSearch({ redirect: '/settings/api-tokens?status=active' })).toEqual({
      redirect: '/settings/api-tokens?status=active',
    })
  })

  it.each([
    undefined,
    null,
    42,
    '',
    'settings/account',
    '//example.com/path',
    '/\\example.com/path',
    'https://example.com/path',
  ])('rejects unsafe redirect value %j', (value) => {
    expect(safeInternalRedirect(value)).toBeUndefined()
    expect(parseLoginSearch({ redirect: value })).toEqual({})
  })

  it('does not nest the login page inside another login redirect', () => {
    expect(loginSearchFor('/login')).toEqual({})
    expect(loginSearchFor('/login?redirect=%2Fsettings')).toEqual({})
  })
})
