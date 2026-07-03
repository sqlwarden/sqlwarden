import { describe, it, expect } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import { isSessionGone } from './sessionErrors'

describe('isSessionGone', () => {
  it('matches a plain 410 (expired/unknown session)', () => {
    expect(isSessionGone(new ApiError('Session has expired or does not exist.', 410))).toBe(true)
  })

  it('does not match cursor-expiry 410s', () => {
    expect(isSessionGone(new ApiError('Cursor gone.', 410, { code: 'query_cursor_unavailable' }))).toBe(false)
  })

  it('does not match other statuses or non-API errors', () => {
    expect(isSessionGone(new ApiError('forbidden', 403))).toBe(false)
    expect(isSessionGone(new Error('boom'))).toBe(false)
    expect(isSessionGone(undefined)).toBe(false)
  })
})
