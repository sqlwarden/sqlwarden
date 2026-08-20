import type { PropsWithChildren } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import type { ResultSet } from '#/lib/api/types'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { useQueryExecution } from './useQueryExecution'

const mocks = vi.hoisted(() => ({
  closeCursor: vi.fn(),
  ensureSession: vi.fn(),
  runQuery: vi.fn(),
  recordHistory: vi.fn(),
}))

vi.mock('#/lib/api/query', () => ({
  closeConnectionQueryCursor: mocks.closeCursor,
  runConnectionQuery: mocks.runQuery,
}))
vi.mock('./sessionErrors', () => ({ useEnsureSession: () => mocks.ensureSession }))
vi.mock('./useHistoryRecorder', () => ({ useHistoryRecorder: () => mocks.recordHistory }))

const result: ResultSet = {
  columns: [{ name: 'id', type: 'integer', raw_type: 'int4', nullable: false }],
  rows: [[{ type: 'integer', integer: 1 }]],
  duration_ms: 4,
  truncated: false,
  rows_returned: 1,
  bytes_returned: 1,
}

describe('useQueryExecution', () => {
  beforeEach(() => {
    mocks.closeCursor.mockReset().mockResolvedValue(undefined)
    mocks.runQuery.mockReset().mockResolvedValue(result)
    mocks.recordHistory.mockReset()
    mocks.ensureSession
      .mockReset()
      .mockImplementation(
        async (_connectionId: number, run: (sessionId: string) => Promise<unknown>) =>
          run('session-1'),
      )
  })

  function wrapper(store: ReturnType<typeof createIdeStore>) {
    return function Wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }
  }

  it('begins a run and executes the statement through the ensured session', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result: hook } = renderHook(() => useQueryExecution('acme', 3, 'query', 7), {
      wrapper: wrapper(store),
    })

    await act(async () => hook.current.run('select 1'))

    expect(mocks.ensureSession).toHaveBeenCalledWith(
      7,
      expect.any(Function),
      expect.any(AbortSignal),
    )
    expect(mocks.runQuery).toHaveBeenCalledWith(
      'acme',
      3,
      7,
      'session-1',
      'select 1',
      expect.objectContaining({ useCursor: true, signal: expect.any(AbortSignal) }),
    )
    const runs = store.getState().resultRuns.query
    expect(runs).toHaveLength(1)
    expect(runs[0].results[0]).toEqual(
      expect.objectContaining({ status: 'ok', sql: 'select 1', connectionId: 7 }),
    )
    expect(store.getState().runningTabs.query).toBe(false)
    expect(store.getState().abortControllers.query).toBeUndefined()
  })

  it('cancels the active request and ignores unavailable or already-running tabs', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const controller = new AbortController()
    store.setState({ abortControllers: { query: controller } })
    const hook = renderHook(() => useQueryExecution('acme', 3, 'query', 7), {
      wrapper: wrapper(store),
    })
    act(() => hook.result.current.cancel())
    expect(controller.signal.aborted).toBe(true)

    store.setState({ runningTabs: { query: true } })
    await waitFor(() => expect(hook.result.current.isRunning).toBe(true))
    await act(async () => hook.result.current.run('select 1'))
    expect(mocks.runQuery).not.toHaveBeenCalled()

    const unavailable = renderHook(() => useQueryExecution('acme', 3, undefined, undefined), {
      wrapper: wrapper(store),
    })
    await act(async () => unavailable.result.current.run('select 1'))
    expect(mocks.ensureSession).not.toHaveBeenCalled()
  })

  it('confirmAt resumes the paused run with confirmUnsafe set', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    mocks.runQuery.mockRejectedValueOnce(
      new ApiError('Confirm to run it anyway.', 422, {
        code: 'unsafe_query_confirmation_required',
        details: [],
      }),
    )
    const { result: hook } = renderHook(() => useQueryExecution('acme', 3, 'query', 7), {
      wrapper: wrapper(store),
    })

    await act(async () => hook.current.run('DELETE FROM widgets'))
    const pending = store.getState().pendingConfirmations.query
    expect(pending).toBeDefined()

    await act(async () => hook.current.confirmAt(0))

    expect(mocks.runQuery).toHaveBeenLastCalledWith(
      'acme',
      3,
      7,
      'session-1',
      'DELETE FROM widgets',
      expect.objectContaining({ confirmUnsafe: true }),
    )
    const runs = store.getState().resultRuns.query
    expect(runs).toHaveLength(1)
    expect(runs[0].results[0]).toEqual(
      expect.objectContaining({ status: 'ok', sql: 'DELETE FROM widgets' }),
    )
  })

  it('confirmAt no-ops when the pending confirmation belongs to a different run', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result: hook } = renderHook(() => useQueryExecution('acme', 3, 'query', 7), {
      wrapper: wrapper(store),
    })

    store.getState().setPendingConfirmation('query', {
      sql: 'select 1',
      statements: [],
      runId: 'someone-elses-run',
      statementIndex: 0,
    })

    await act(async () => hook.current.confirmAt(0))

    expect(mocks.runQuery).not.toHaveBeenCalled()
  })
})
