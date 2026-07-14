// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AUTH_INVALIDATED_EVENT } from '#/lib/auth/invalidation'
import { setAccessToken } from '#/lib/auth/access-token'
import { errorMessage } from '#/lib/api/errors'
import { apiRequest, buildSearchParams, parseAPIErrorPayload } from './client'

describe('buildSearchParams', () => {
  it('omits empty and undefined values while preserving meaningful falsy values', () => {
    const params = buildSearchParams({
      q: '',
      page: 0,
      page_size: 25,
      sort: undefined,
      include_inherited: false,
    })

    expect(params.toString()).toBe('page=0&page_size=25&include_inherited=false')
  })
})

describe('errorMessage', () => {
  it('uses an Error message and otherwise returns the fallback', () => {
    expect(errorMessage(new Error('Database unavailable.'), 'Fallback')).toBe(
      'Database unavailable.',
    )
    expect(errorMessage({ message: 'not trusted' }, 'Fallback')).toBe('Fallback')
    expect(errorMessage(new Error(''), 'Fallback')).toBe('Fallback')
  })
})

describe('parseAPIErrorPayload', () => {
  it('reads the standard error envelope and prefers a field validation message', () => {
    expect(
      parseAPIErrorPayload(
        {
          error: {
            code: 'validation_failed',
            message: 'Validation failed.',
            field_errors: { email: 'Email address is already in use.' },
            details: { field: 'email' },
          },
        },
        'Fallback',
      ),
    ).toEqual({
      code: 'validation_failed',
      details: { field: 'email' },
      fieldErrors: { email: 'Email address is already in use.' },
      message: 'Email address is already in use.',
    })
  })

  it('supports legacy field errors and string errors', () => {
    expect(
      parseAPIErrorPayload({ field_errors: { name: 'Name is required.' } }, 'Fallback'),
    ).toMatchObject({
      fieldErrors: { name: 'Name is required.' },
      message: 'Name is required.',
    })
    expect(parseAPIErrorPayload({ error: 'Access denied.' }, 'Fallback').message).toBe(
      'Access denied.',
    )
  })

  it('uses the fallback for an unknown payload', () => {
    expect(parseAPIErrorPayload(null, 'Request failed safely.')).toEqual({
      code: undefined,
      details: null,
      fieldErrors: undefined,
      message: 'Request failed safely.',
    })
  })
})

describe('apiRequest', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('builds an authenticated JSON request with query parameters', async () => {
    setAccessToken('token-123')
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ id: 7 }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    )

    await expect(
      apiRequest<{ id: number }>('/api/items', {
        method: 'POST',
        query: { page: 2, q: 'orders' },
        body: { name: 'Report' },
      }),
    ).resolves.toEqual({ id: 7 })

    const [url, init] = vi.mocked(fetch).mock.calls[0]!
    expect(String(url)).toBe(`${window.location.origin}/api/items?page=2&q=orders`)
    expect(init?.body).toBe(JSON.stringify({ name: 'Report' }))
    const headers = new Headers(init?.headers)
    expect(headers.get('authorization')).toBe('Bearer token-123')
    expect(headers.get('content-type')).toBe('application/json')
  })

  it('clears authentication and emits invalidation after an authenticated 401', async () => {
    setAccessToken('expired-token')
    const invalidated = vi.fn()
    window.addEventListener(AUTH_INVALIDATED_EVENT, invalidated, { once: true })
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          error: { code: 'unauthorized', message: 'Session expired.' },
        }),
        {
          status: 401,
          statusText: 'Unauthorized',
          headers: { 'content-type': 'application/json' },
        },
      ),
    )

    await expect(apiRequest('/api/v1/session')).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: 'Session expired.',
    })
    expect(window.localStorage.getItem('sqlwarden.access_token')).toBeNull()
    expect(invalidated).toHaveBeenCalledOnce()
  })

  it('returns undefined for an empty successful response', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }))
    await expect(apiRequest('/api/items/1', { method: 'DELETE' })).resolves.toBeUndefined()
  })
})
