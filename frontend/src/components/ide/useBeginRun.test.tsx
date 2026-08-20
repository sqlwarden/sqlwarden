import type { PropsWithChildren } from 'react'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { RUN_HISTORY_CAP } from './resultRunHistory'
import { useBeginRun } from './useBeginRun'

const mocks = vi.hoisted(() => ({ closeCursor: vi.fn() }))
vi.mock('#/lib/api/query', () => ({ closeConnectionQueryCursor: mocks.closeCursor }))

describe('useBeginRun', () => {
  beforeEach(() => {
    mocks.closeCursor.mockReset().mockResolvedValue(undefined)
  })

  function wrapper(store: ReturnType<typeof createIdeStore>) {
    return function Wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }
  }

  it('begins a run in the store and returns its id', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result } = renderHook(() => useBeginRun('acme', 3, 'tab-1', 7), {
      wrapper: wrapper(store),
    })

    let runId = ''
    act(() => {
      runId = result.current(['select 1'])
    })

    expect(store.getState().resultRuns['tab-1']).toHaveLength(1)
    expect(store.getState().resultRuns['tab-1'][0].id).toBe(runId)
  })

  it('stamps the run with the connection it started against', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result } = renderHook(() => useBeginRun('acme', 3, 'tab-1', 7), {
      wrapper: wrapper(store),
    })

    act(() => {
      result.current(['select 1'])
    })

    expect(store.getState().resultRuns['tab-1'][0].connectionId).toBe(7)
  })

  it('evicts the oldest run and closes its cursors once the tab exceeds the history cap', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const { result } = renderHook(() => useBeginRun('acme', 3, 'tab-1', 7), {
      wrapper: wrapper(store),
    })

    let firstRunId = ''
    act(() => {
      firstRunId = result.current(['select 1'])
    })
    store.getState().setRunStatementResult('tab-1', firstRunId, 0, {
      status: 'ok',
      sql: 'select 1',
      durationMs: 1,
      connectionId: 9,
      data: {
        columns: [],
        rows: [],
        duration_ms: 1,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
        query_cursor_id: 'cursor-1',
      },
    })

    for (let i = 0; i < RUN_HISTORY_CAP; i++) {
      act(() => {
        result.current(['select 1'])
      })
    }

    expect(store.getState().resultRuns['tab-1']).toHaveLength(RUN_HISTORY_CAP)
    expect(store.getState().resultRuns['tab-1'].some((r) => r.id === firstRunId)).toBe(false)
    expect(mocks.closeCursor).toHaveBeenCalledWith('acme', 3, 9, 'cursor-1')
  })
})
