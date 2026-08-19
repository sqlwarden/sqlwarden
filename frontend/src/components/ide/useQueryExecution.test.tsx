import type { PropsWithChildren } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ResultSet } from '#/lib/api/types'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { useQueryExecution } from './useQueryExecution'

const mocks = vi.hoisted(() => ({
  closeCursor: vi.fn(),
  ensureSession: vi.fn(),
  runQuery: vi.fn(),
}))

vi.mock('#/lib/api/query', () => ({
  closeConnectionQueryCursor: mocks.closeCursor,
  runConnectionQuery: mocks.runQuery,
}))
vi.mock('./sessionErrors', () => ({ useEnsureSession: () => mocks.ensureSession }))

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
    mocks.ensureSession
      .mockReset()
      .mockImplementation(
        async (_connectionId: number, run: (sessionId: string) => Promise<unknown>) =>
          run('session-1'),
      )
  })

  it('closes the previous cursor and executes through the ensured session', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    store.setState({
      results: {
        query: [
          {
            status: 'ok',
            sql: 'select old',
            durationMs: 1,
            connectionId: 8,
            data: { ...result, query_cursor_id: 'cursor-1' },
          },
        ],
      },
    })
    function wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }
    const { result: hook } = renderHook(() => useQueryExecution('acme', 3, 'query', 7), { wrapper })

    await act(async () => hook.current.run('select 1'))

    expect(mocks.closeCursor).toHaveBeenCalledWith('acme', 3, 8, 'cursor-1')
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
    expect(store.getState().results.query[0]).toEqual(
      expect.objectContaining({
        status: 'ok',
        sql: 'select 1',
        connectionId: 7,
      }),
    )
    expect(store.getState().runningTabs.query).toBe(false)
    expect(store.getState().abortControllers.query).toBeUndefined()
  })

  it('cancels the active request and ignores unavailable or already-running tabs', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const controller = new AbortController()
    store.setState({ abortControllers: { query: controller } })
    function wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }
    const hook = renderHook(() => useQueryExecution('acme', 3, 'query', 7), { wrapper })
    act(() => hook.result.current.cancel())
    expect(controller.signal.aborted).toBe(true)

    store.setState({ runningTabs: { query: true } })
    await waitFor(() => expect(hook.result.current.isRunning).toBe(true))
    await act(async () => hook.result.current.run('select 1'))
    expect(mocks.runQuery).not.toHaveBeenCalled()

    const unavailable = renderHook(() => useQueryExecution('acme', 3, undefined, undefined), {
      wrapper,
    })
    await act(async () => unavailable.result.current.run('select 1'))
    expect(mocks.ensureSession).not.toHaveBeenCalled()
  })
})
