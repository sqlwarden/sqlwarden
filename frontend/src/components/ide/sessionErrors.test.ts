import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import {
  ensureSession,
  isSessionGone,
  TransactionSessionLostError,
  type EnsureSessionDeps,
} from './sessionErrors'

describe('isSessionGone', () => {
  it('matches a plain 410 (expired/unknown session)', () => {
    expect(isSessionGone(new ApiError('Session has expired or does not exist.', 410))).toBe(true)
  })

  it('does not match cursor-expiry 410s', () => {
    expect(
      isSessionGone(new ApiError('Cursor gone.', 410, { code: 'query_cursor_unavailable' })),
    ).toBe(false)
  })

  it('does not match other statuses or non-API errors', () => {
    expect(isSessionGone(new ApiError('forbidden', 403))).toBe(false)
    expect(isSessionGone(new Error('boom'))).toBe(false)
    expect(isSessionGone(undefined)).toBe(false)
  })
})

function makeDeps(overrides: Partial<EnsureSessionDeps> = {}): EnsureSessionDeps {
  return {
    getSession: vi.fn(() => undefined),
    setSession: vi.fn(),
    clearSession: vi.fn(),
    setConnectionStatus: vi.fn(),
    connect: vi.fn(async () => 'new-session-id'),
    wasManualTransaction: vi.fn(() => false),
    resetTransactionState: vi.fn(),
    ...overrides,
  }
}

describe('ensureSession', () => {
  it('reuses a cached session without connecting', async () => {
    const deps = makeDeps({ getSession: vi.fn(() => 'cached-session') })
    const run = vi.fn(async (sessionId: string) => `ran-with-${sessionId}`)

    const result = await ensureSession(deps, 7, run)

    expect(result).toBe('ran-with-cached-session')
    expect(deps.connect).not.toHaveBeenCalled()
    expect(run).toHaveBeenCalledWith('cached-session')
  })

  it('connects when there is no cached session and stores the new session', async () => {
    const deps = makeDeps()
    const run = vi.fn(async (sessionId: string) => `ran-with-${sessionId}`)

    const result = await ensureSession(deps, 7, run)

    expect(result).toBe('ran-with-new-session-id')
    expect(deps.connect).toHaveBeenCalledWith(7, undefined)
    expect(deps.setSession).toHaveBeenCalledWith(7, 'new-session-id')
    expect(deps.setConnectionStatus).toHaveBeenNthCalledWith(1, 7, 'connecting')
    expect(deps.setConnectionStatus).toHaveBeenNthCalledWith(2, 7, null)
  })

  it('clears the session and retries once when run fails with a gone session', async () => {
    const deps = makeDeps({ getSession: vi.fn(() => 'dead-session') })
    const goneError = new ApiError('gone', 410)
    const run = vi.fn().mockRejectedValueOnce(goneError).mockResolvedValueOnce('ok-after-retry')

    const result = await ensureSession(deps, 7, run)

    expect(result).toBe('ok-after-retry')
    expect(deps.clearSession).toHaveBeenCalledWith(7)
    expect(deps.connect).toHaveBeenCalledTimes(1)
    expect(run).toHaveBeenCalledTimes(2)
    expect(run).toHaveBeenNthCalledWith(1, 'dead-session')
    expect(run).toHaveBeenNthCalledWith(2, 'new-session-id')
  })

  it('does not retry a second time and rethrows non-session errors', async () => {
    const deps = makeDeps({ getSession: vi.fn(() => 'cached-session') })
    const err = new Error('boom')
    const run = vi.fn().mockRejectedValue(err)

    await expect(ensureSession(deps, 7, run)).rejects.toBe(err)
    expect(deps.clearSession).not.toHaveBeenCalled()
  })

  it('does not retry when the dead session was in manual transaction mode', async () => {
    const deps = makeDeps({
      getSession: vi.fn(() => 'dead-session'),
      wasManualTransaction: vi.fn(() => true),
    })
    const goneError = new ApiError('gone', 410)
    const run = vi.fn().mockRejectedValue(goneError)

    await expect(ensureSession(deps, 7, run)).rejects.toBeInstanceOf(TransactionSessionLostError)
    expect(deps.clearSession).toHaveBeenCalledWith(7)
    expect(deps.resetTransactionState).toHaveBeenCalledWith(7)
    expect(deps.connect).not.toHaveBeenCalled()
    expect(run).toHaveBeenCalledTimes(1)
  })

  it('refuses to silently connect when no session is cached but stale state says manual', async () => {
    const deps = makeDeps({
      getSession: vi.fn(() => undefined),
      wasManualTransaction: vi.fn(() => true),
    })
    const run = vi.fn()

    await expect(ensureSession(deps, 7, run)).rejects.toBeInstanceOf(TransactionSessionLostError)
    expect(deps.resetTransactionState).toHaveBeenCalledWith(7)
    expect(deps.connect).not.toHaveBeenCalled()
    expect(run).not.toHaveBeenCalled()
  })
})
