import type { PropsWithChildren } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext, type QueryResult } from './useIdeStore'
import { useResultCursorPaging } from './useResultCursorPaging'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const initialResult: Extract<QueryResult, { status: 'ok' }> = {
  status: 'ok',
  durationMs: 4,
  sql: 'select * from users',
  connectionId: 7,
  data: {
    columns: [],
    rows: [[{ type: 'integer', integer: 1 }]],
    duration_ms: 4,
    truncated: true,
    rows_returned: 1,
    bytes_returned: 8,
    query_cursor_id: 'cursor-1',
    exhausted: false,
    page_size: 20,
  },
}

describe('useResultCursorPaging', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('test', 1, 'ephemeral')
    store.getState().setQueryResult('tab-1', initialResult)
  })

  function wrapper({ children }: PropsWithChildren) {
    return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
  }

  function renderPaging(result = initialResult) {
    return renderHook(
      () =>
        useResultCursorPaging({
          activeTabId: 'tab-1',
          connectionId: 7,
          orgSlug: 'acme',
          result,
          workspaceId: 3,
        }),
      { wrapper },
    )
  }

  it('merges a successful page and retires an exhausted cursor', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/query-cursors/cursor-1/fetch', () =>
        HttpResponse.json({
          ...initialResult.data,
          rows: [[{ type: 'integer', integer: 2 }]],
          rows_returned: 1,
          bytes_returned: 9,
          exhausted: true,
        }),
      ),
    )
    const { result } = renderPaging()

    await act(() => result.current.fetchNextPage())

    const stored = store.getState().results['tab-1']
    expect(stored.status).toBe('ok')
    if (stored.status !== 'ok') return
    expect(stored.data.rows).toHaveLength(2)
    expect(stored.data.query_cursor_id).toBeUndefined()
    expect(stored.data.exhausted).toBe(true)
  })

  it('deduplicates concurrent page requests', async () => {
    let requests = 0
    let resolveRequest: (() => void) | undefined
    server.use(
      http.post(
        '/api/v1/orgs/acme/workspaces/3/connections/7/query-cursors/cursor-1/fetch',
        async () => {
          requests += 1
          await new Promise<void>((resolve) => {
            resolveRequest = resolve
          })
          return HttpResponse.json({ ...initialResult.data, rows: [], exhausted: true })
        },
      ),
    )
    const { result } = renderPaging()

    let first: Promise<void>
    await act(async () => {
      first = result.current.fetchNextPage()
      void result.current.fetchNextPage()
      await waitFor(() => expect(requests).toBe(1))
      resolveRequest?.()
      await first
    })

    expect(requests).toBe(1)
  })

  it('retires an expired cursor with an actionable message', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/query-cursors/cursor-1/fetch', () =>
        HttpResponse.json(
          { error: { code: 'query_cursor_unavailable', message: 'Cursor unavailable.' } },
          { status: 410 },
        ),
      ),
    )
    const { result } = renderPaging()

    await act(() => result.current.fetchNextPage())

    const stored = store.getState().results['tab-1']
    expect(stored.status).toBe('ok')
    if (stored.status !== 'ok') return
    expect(stored.cursorMessage).toBe('Cursor expired. Run the query again.')
    expect(stored.data.query_cursor_id).toBeUndefined()
  })
})
