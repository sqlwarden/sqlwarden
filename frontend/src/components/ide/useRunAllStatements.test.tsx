import type { PropsWithChildren } from 'react'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import type { ResultSet } from '#/lib/api/types'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { useRunAllStatements } from './useRunAllStatements'

const mocks = vi.hoisted(() => ({
  ensureSession: vi.fn(),
  runQuery: vi.fn(),
  recordHistory: vi.fn(),
}))

vi.mock('#/lib/api/query', () => ({
  runConnectionQuery: mocks.runQuery,
}))
vi.mock('./sessionErrors', () => ({ useEnsureSession: () => mocks.ensureSession }))
vi.mock('./useHistoryRecorder', () => ({ useHistoryRecorder: () => mocks.recordHistory }))

const result: ResultSet = {
  columns: [],
  rows: [],
  duration_ms: 3,
  truncated: false,
  rows_returned: 0,
  bytes_returned: 0,
}

describe('useRunAllStatements', () => {
  beforeEach(() => {
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

  it('runs every statement in order and stores one ok result per index', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result: hook } = renderHook(() => useRunAllStatements('acme', 3, 'tab-1', 7), {
      wrapper: wrapper(store),
    })

    await act(async () => hook.current.runAll(['select 1', 'select 2']))

    expect(mocks.runQuery).toHaveBeenCalledTimes(2)
    expect(mocks.runQuery).toHaveBeenNthCalledWith(
      1,
      'acme',
      3,
      7,
      'session-1',
      'select 1',
      expect.objectContaining({ useCursor: true, confirmUnsafe: false }),
    )
    const stored = store.getState().results['tab-1']
    expect(stored).toHaveLength(2)
    expect(stored[0]).toEqual(expect.objectContaining({ status: 'ok', sql: 'select 1' }))
    expect(stored[1]).toEqual(expect.objectContaining({ status: 'ok', sql: 'select 2' }))
    expect(store.getState().runningTabs['tab-1']).toBe(false)
  })

  it('resumes a paused batch from confirmAt using the last-seen statements', async () => {
    mocks.runQuery.mockRejectedValueOnce(
      new ApiError('Confirm to run it anyway.', 422, {
        code: 'unsafe_query_confirmation_required',
        details: [],
      }),
    )
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result: hook } = renderHook(() => useRunAllStatements('acme', 3, 'tab-1', 7), {
      wrapper: wrapper(store),
    })

    await act(async () => hook.current.runAll(['DELETE FROM widgets', 'select 2']))
    expect(store.getState().pendingConfirmations['tab-1']).toEqual(
      expect.objectContaining({ batchIndex: 0 }),
    )

    mocks.runQuery.mockResolvedValue(result)
    await act(async () => hook.current.confirmAt(0))

    expect(mocks.runQuery).toHaveBeenLastCalledWith(
      'acme',
      3,
      7,
      'session-1',
      'select 2',
      expect.objectContaining({ useCursor: true, confirmUnsafe: false }),
    )
    const stored = store.getState().results['tab-1']
    expect(stored[0]).toEqual(expect.objectContaining({ status: 'ok', sql: 'DELETE FROM widgets' }))
    expect(stored[1]).toEqual(expect.objectContaining({ status: 'ok', sql: 'select 2' }))
  })

  it('cancels the in-flight batch by aborting its controller', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const controller = new AbortController()
    store.setState({ abortControllers: { 'tab-1': controller } })
    const { result: hook } = renderHook(() => useRunAllStatements('acme', 3, 'tab-1', 7), {
      wrapper: wrapper(store),
    })

    act(() => hook.current.cancel())

    expect(controller.signal.aborted).toBe(true)
  })
})
