import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

  function renderResultsArea() {
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

  function beginRunWithResult(result: QueryResult, connectionId?: number) {
    const runId = store.getState().beginRun('scratch-1', ['placeholder'], connectionId)
    store.getState().setRunStatementResult('scratch-1', runId, 0, result)
    return runId
  }

  function renderResult(result: QueryResult, connectionId?: number) {
    beginRunWithResult(result, connectionId)
    return renderResultsArea()
  }

  it.each([
    [{ status: 'idle' } as const, 'Run a query to see results'],
    [{ status: 'running', sql: 'select 1' } as const, 'Running query…'],
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

  it('renders the affected row count for an executed statement', async () => {
    renderResult({
      status: 'ok',
      durationMs: 8,
      sql: 'delete from users where disabled = true',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        rows_affected: 3,
        duration_ms: 8,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
      },
    })

    expect(await screen.findByText('· 3 rows affected')).toBeInTheDocument()
  })

  it('renders zero affected rows as a reported result', async () => {
    renderResult({
      status: 'ok',
      durationMs: 4,
      sql: 'update users set active = true where false',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        rows_affected: 0,
        duration_ms: 4,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
      },
    })

    expect(await screen.findByText('· 0 rows affected')).toBeInTheDocument()
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

  it('shows the connection name in the bottom status bar, before the row count, not in the sql caption', async () => {
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

    const rowsLabel = await screen.findByText('0 rows')
    const statusBar = rowsLabel.parentElement as HTMLElement
    const connectionLabel = await within(statusBar).findByText('analytics-pg')
    expect(
      connectionLabel.compareDocumentPosition(rowsLabel) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()

    const caption = screen.getByTitle('select id from users where false').closest('div')!
    expect(within(caption).queryByText('analytics-pg')).not.toBeInTheDocument()
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

  it('lists every statement in a sidebar and shows the first one selected by default', async () => {
    const runId = store
      .getState()
      .beginRun('scratch-1', ['select 1', 'select 2', 'select 3', 'select 4'])
    store.getState().setRunStatementResult('scratch-1', runId, 0, {
      status: 'ok',
      durationMs: 2,
      sql: 'select 1',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        duration_ms: 2,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
        rows_affected: 2,
      },
    })
    store.getState().setRunStatementResult('scratch-1', runId, 1, {
      status: 'error',
      message: 'boom',
      sql: 'select 2',
    })
    store
      .getState()
      .setRunStatementResult('scratch-1', runId, 2, { status: 'skipped', sql: 'select 3' })

    renderResultsArea()

    const list = await screen.findByRole('listbox', { name: 'Statement results' })
    expect(within(list).getByText('Queued')).toBeInTheDocument()
    expect(within(list).getByText('Failed')).toBeInTheDocument()
    expect(within(list).getByText('Skipped')).toBeInTheDocument()
    // The first statement (ok) is selected by default and shown in the detail pane.
    expect(screen.getByText('· 2 rows affected')).toBeInTheDocument()
    expect(screen.queryByText('Query failed')).not.toBeInTheDocument()
  })

  it('switches the detail pane when a different statement row is clicked', async () => {
    const user = userEvent.setup()
    const runId = store.getState().beginRun('scratch-1', ['select 1', 'select 2'])
    store.getState().setRunStatementResult('scratch-1', runId, 0, {
      status: 'ok',
      durationMs: 2,
      sql: 'select 1',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        duration_ms: 2,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
        rows_affected: 2,
      },
    })
    store.getState().setRunStatementResult('scratch-1', runId, 1, {
      status: 'error',
      message: 'boom',
      sql: 'select 2',
    })

    renderResultsArea()

    await screen.findByText('· 2 rows affected')
    await user.click(screen.getByRole('option', { name: /select 2/ }))

    expect(screen.getByText('Query failed')).toBeInTheDocument()
    expect(screen.queryByText('· 2 rows affected')).not.toBeInTheDocument()
  })

  it('shows a run tab even when there is only one run', async () => {
    beginRunWithResult({
      status: 'ok',
      durationMs: 2,
      sql: 'select 1',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        duration_ms: 2,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
        rows_affected: 1,
      },
    })

    renderResultsArea()

    const runTabs = within(await screen.findByRole('tablist', { name: 'Runs' }))
    expect(await runTabs.findAllByRole('tab')).toHaveLength(1)
  })

  it('labels a running run tab with its query instead of a generic run number', async () => {
    const runId = store.getState().beginRun('scratch-1', ['select 1'], 7)
    store.getState().setRunStatementResult('scratch-1', runId, 0, {
      status: 'running',
      sql: 'select 1',
    })

    renderResultsArea()

    const runTabs = within(await screen.findByRole('tablist', { name: 'Runs' }))
    expect(await runTabs.findByText('select 1')).toBeInTheDocument()
    expect(runTabs.queryByText('Run 1')).not.toBeInTheDocument()
  })

  it('shows one tab per run and switches results when a run tab is clicked', async () => {
    const user = userEvent.setup()
    beginRunWithResult({
      status: 'ok',
      durationMs: 2,
      sql: 'select 1',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        duration_ms: 2,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
        rows_affected: 1,
      },
    })
    beginRunWithResult({ status: 'error', sql: 'select 2', message: 'boom' })

    renderResultsArea()

    const runTabs = within(await screen.findByRole('tablist', { name: 'Runs' }))
    const tabs = await runTabs.findAllByRole('tab')
    expect(tabs).toHaveLength(2)
    // The most recently begun run is selected by default.
    expect(screen.getByText('Query failed')).toBeInTheDocument()

    await user.click(tabs[0])
    expect(screen.getByText('· 1 row affected')).toBeInTheDocument()
  })

  it('closes a run tab and falls back to the previous run', async () => {
    const user = userEvent.setup()
    beginRunWithResult({
      status: 'ok',
      durationMs: 2,
      sql: 'select 1',
      connectionId: 7,
      data: {
        columns: [],
        rows: [],
        duration_ms: 2,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
        rows_affected: 1,
      },
    })
    beginRunWithResult({ status: 'error', sql: 'select 2', message: 'boom' })

    renderResultsArea()
    expect(screen.getByText('Query failed')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Close run 2' }))

    expect(store.getState().resultRuns['scratch-1']).toHaveLength(1)
    expect(screen.getByText('· 1 row affected')).toBeInTheDocument()
  })

  it('shows the connection a failed query ran against', async () => {
    renderResult({ status: 'error', sql: 'select 1', message: 'permission denied' }, 7)

    expect(await screen.findByText('Query failed')).toBeInTheDocument()
    expect(await screen.findByText('analytics-pg')).toBeInTheDocument()
  })

  it("labels each run tab with its own connection's driver badge", async () => {
    beginRunWithResult(
      {
        status: 'ok',
        durationMs: 2,
        sql: 'select 1',
        connectionId: 7,
        data: {
          columns: [],
          rows: [],
          duration_ms: 2,
          truncated: false,
          rows_returned: 0,
          bytes_returned: 0,
          rows_affected: 1,
        },
      },
      7,
    )
    beginRunWithResult({ status: 'error', sql: 'select 2', message: 'boom' }, 7)

    renderResultsArea()

    const runTabs = within(await screen.findByRole('tablist', { name: 'Runs' }))
    expect(await runTabs.findAllByRole('img', { name: 'PostgreSQL' })).toHaveLength(2)
  })
})
