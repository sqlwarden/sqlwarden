import { describe, it, expect } from 'vitest'
import { resolveObjectViewState } from './viewState'
import { ApiError } from '#/lib/api/errors'

const api = (status: number) => new ApiError('m', status)

describe('resolveObjectViewState', () => {
  it('no session -> no-session (even mid-load)', () => {
    expect(resolveObjectViewState({ hasSession: false, isLoading: true, error: null, hasData: false }))
      .toEqual({ kind: 'no-session' })
  })
  it('loading with session and no data -> loading', () => {
    expect(resolveObjectViewState({ hasSession: true, isLoading: true, error: null, hasData: false }))
      .toEqual({ kind: 'loading' })
  })
  it('501 -> unsupported', () => {
    expect(resolveObjectViewState({ hasSession: true, isLoading: false, error: api(501), hasData: false }))
      .toEqual({ kind: 'unsupported' })
  })
  it('403 -> forbidden', () => {
    expect(resolveObjectViewState({ hasSession: true, isLoading: false, error: api(403), hasData: false }))
      .toEqual({ kind: 'forbidden' })
  })
  it('410 -> no-session (connection gone)', () => {
    expect(resolveObjectViewState({ hasSession: true, isLoading: false, error: api(410), hasData: false }))
      .toEqual({ kind: 'no-session' })
  })
  it('other error -> error with message', () => {
    expect(resolveObjectViewState({ hasSession: true, isLoading: false, error: new Error('boom'), hasData: false }))
      .toEqual({ kind: 'error', message: 'boom' })
  })
  it('data present -> ready (even while refetching)', () => {
    expect(resolveObjectViewState({ hasSession: true, isLoading: true, error: null, hasData: true }))
      .toEqual({ kind: 'ready' })
  })
})
