import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { QueryResult } from './useIdeStore'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { ResultsArea } from './ResultsArea'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org' as const,
  owner_id: 1,
  name: 'Analytics',
  environment_count: 1,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}

describe('ResultsArea', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    store.getState().setActiveWorkspace(workspace.id)
    store.getState().openTab({
      id: 'scratch-1',
      workspaceId: workspace.id,
      title: 'Console 1',
      kind: 'scratch',
      content: 'select 1',
      connectionId: 7,
    })
    server.use(http.get(
      '/api/v1/orgs/acme/workspaces/3/connections',
      () => HttpResponse.json({
        items: [{
          id: 7,
          workspace_id: 3,
          environment_id: 2,
          name: 'analytics-pg',
          driver: 'postgres',
          access_mode: 'open',
          created_at: '',
          updated_at: '',
        }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    ))
  })

  function renderResult(result: QueryResult) {
    store.getState().setQueryResult('scratch-1', result)
    const queryClient = createTestQueryClient()
    return render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <ResultsArea orgSlug="acme" workspace={workspace} />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  it.each([
    [{ status: 'idle' } as const, 'Run a query to see results'],
    [{ status: 'running' } as const, 'Running query…'],
    [{ status: 'cancelled', sql: 'select 1' } as const, 'Query cancelled'],
    [{ status: 'error', sql: 'select 1', message: 'permission denied' } as const, 'Query failed'],
  ])('renders the %s result state', async (result, message) => {
    renderResult(result)
    expect(await screen.findByText(message)).toBeInTheDocument()
  })

  it('renders an execution summary when a query returns no columns', async () => {
    renderResult({
      status: 'ok',
      durationMs: 12,
      sql: 'update users set active = true',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        duration_ms: 12,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
      },
    })

    expect(await screen.findByText('Query executed')).toBeInTheDocument()
    expect(screen.getByText('· 12ms')).toBeInTheDocument()
    expect(screen.getByTitle('update users set active = true')).toBeInTheDocument()
  })

  it('renders an empty grid and result metadata for a column-bearing query', async () => {
    renderResult({
      status: 'ok',
      durationMs: 3,
      sql: 'select id from users where false',
      connectionId: 7,
      data: {
        columns: [{ name: 'id', type: 'integer', raw_type: 'int4', nullable: false }],
        rows: [],
        duration_ms: 3,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
      },
    })

    expect(await screen.findByText('No rows returned')).toBeInTheDocument()
    expect(screen.getByText('0 rows')).toBeInTheDocument()
    expect(await screen.findByText('analytics-pg')).toBeInTheDocument()
  })
})
