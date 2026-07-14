import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { QueryResult } from './useIdeStore'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { ResultsArea } from './ResultsArea'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { ContextMenuProvider } from '#/components/ui/context-menu'

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 29,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        start: index * 29,
        end: (index + 1) * 29,
        size: 29,
        key: index,
        lane: 0,
      })),
    scrollToIndex: vi.fn(),
  }),
}))

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
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({
          items: [
            {
              id: 7,
              workspace_id: 3,
              environment_id: 2,
              name: 'analytics-pg',
              driver: 'postgres',
              access_mode: 'open',
              created_at: '',
              updated_at: '',
            },
          ],
          page: 1,
          page_size: 100,
          total: 1,
        }),
      ),
    )
  })

  function renderResult(result: QueryResult) {
    store.getState().setQueryResult('scratch-1', result)
    const queryClient = createTestQueryClient()
    return render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <ContextMenuProvider>
            <ResultsArea orgSlug="acme" workspace={workspace} />
          </ContextMenuProvider>
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

  function populatedResult(): QueryResult {
    return {
      status: 'ok',
      durationMs: 5,
      sql: 'select name, age from users',
      connectionId: 7,
      data: {
        columns: [
          { name: 'name', type: 'text', raw_type: 'text', nullable: false },
          { name: 'age', type: 'integer', raw_type: 'int4', nullable: false },
        ],
        rows: [
          [
            { type: 'text', text: 'Alice' },
            { type: 'integer', integer: 30 },
          ],
          [
            { type: 'text', text: 'Bob' },
            { type: 'integer', integer: 41 },
          ],
        ],
        duration_ms: 5,
        truncated: false,
        rows_returned: 2,
        bytes_returned: 24,
      },
    }
  }

  it('opens cell details and follows keyboard navigation through the grid', async () => {
    renderResult(populatedResult())
    const alice = await screen.findByRole('gridcell', { name: 'Alice' })

    fireEvent.mouseDown(alice, { button: 0 })
    fireEvent.mouseUp(window)
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
    expect(screen.getByText('Alice', { selector: 'pre' })).toBeInTheDocument()

    fireEvent.keyDown(alice, { key: 'ArrowRight' })
    expect(await screen.findByText('30', { selector: 'pre' })).toBeInTheDocument()
  })

  it('copies the selected grid range with the keyboard shortcut', async () => {
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
    renderResult(populatedResult())
    const alice = await screen.findByRole('gridcell', { name: 'Alice' })

    fireEvent.mouseDown(alice, { button: 0 })
    fireEvent.mouseUp(window)
    fireEvent.keyDown(alice, { key: 'c', ctrlKey: true })

    expect(clipboard).toHaveBeenCalledWith('Alice')
  })

  it('routes cell context-menu actions through the shared menu provider', async () => {
    const user = userEvent.setup()
    renderResult(populatedResult())
    fireEvent.contextMenu(await screen.findByRole('gridcell', { name: 'Alice' }))

    await user.click(await screen.findByRole('menuitem', { name: 'Copy' }))
    await waitFor(() =>
      expect(screen.queryByRole('menuitem', { name: 'Copy' })).not.toBeInTheDocument(),
    )
  })

  it('maximizes, restores, and hides the results pane', async () => {
    const user = userEvent.setup()
    renderResult({ status: 'idle' })
    const toggle = screen.getByRole('button', { name: 'Toggle results maximize' })

    await user.click(toggle)
    expect(store.getState().maximizedPane).toBe('results')
    await user.click(toggle)
    expect(store.getState().maximizedPane).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Hide results panel' }))
    expect(store.getState().maximizedPane).toBe('editor')
  })
})
