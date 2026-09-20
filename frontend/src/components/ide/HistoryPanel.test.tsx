import { QueryClientProvider } from '@tanstack/react-query'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { organizationRuntimeSettingsFixture } from '#/test/fixtures'
import { server } from '#/test/server'
import { HistoryPanel } from './HistoryPanel'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { createIdeStore, IdeStoreContext, type EditorTab } from './useIdeStore'

const { copyWithToastMock, createFavoriteMock, removeFavoriteMock, toastMock } = vi.hoisted(() => ({
  copyWithToastMock: vi.fn(),
  createFavoriteMock: vi.fn().mockResolvedValue(undefined),
  removeFavoriteMock: vi.fn().mockResolvedValue(undefined),
  toastMock: vi.fn(),
}))

vi.mock('./object-detail/ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => <pre>{value}</pre>,
}))

vi.mock('sonner', () => ({ toast: toastMock }))

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({
    count,
    estimateSize,
  }: {
    count: number
    estimateSize: (index: number) => number
  }) => {
    let offset = 0
    const items = Array.from({ length: count }, (_, index) => {
      const size = estimateSize(index)
      const item = { index, start: offset, end: offset + size, size, key: index, lane: 0 }
      offset += size
      return item
    })
    return {
      getTotalSize: () => offset,
      getVirtualItems: () => items,
      scrollToIndex: vi.fn(),
    }
  },
}))

vi.mock('./contextMenus/clipboard', () => ({ copyWithToast: copyWithToastMock }))
vi.mock('./useFavoritesMutations', () => ({
  useFavoritesMutations: () => ({
    create: createFavoriteMock,
    update: vi.fn(),
    remove: removeFavoriteMock,
  }),
}))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 1,
  connection_count: 2,
  created_at: '',
  updated_at: '',
}

const scratchTab: EditorTab = {
  id: 'scratch:3:1',
  workspaceId: 3,
  title: 'Console 1',
  kind: 'scratch',
  content: '',
  connectionId: 42,
  driver: 'postgres',
}

type HistoryFixture = {
  id: number
  connectionId: number
  sqlText: string
}

function sqlText(value: string) {
  return (_content: string, element: Element | null) => {
    if (!element || element.textContent !== value) return false
    return Array.from(element.children).every((child) => child.textContent !== value)
  }
}

function entryFor(fixture: HistoryFixture) {
  return {
    id: fixture.id,
    connection_id: fixture.connectionId,
    account_id: 1,
    sql_text: fixture.sqlText,
    status: 'ok' as const,
    error_message: null,
    duration_ms: 5,
    rows_affected: 1,
    executed_at: new Date().toISOString(),
  }
}

describe('HistoryPanel', () => {
  let store: ReturnType<typeof createIdeStore>
  let views: ReturnType<typeof createEditorViewRegistry>

  beforeEach(() => {
    copyWithToastMock.mockClear()
    createFavoriteMock.mockClear()
    removeFavoriteMock.mockClear()
    store = createIdeStore('acme', 1, 'ephemeral')
    views = createEditorViewRegistry()
    server.use(
      http.get('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json(organizationRuntimeSettingsFixture()),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({
          items: [
            {
              id: 42,
              workspace_id: 3,
              environment_id: 2,
              name: 'primary-pg',
              driver: 'postgres',
              access_mode: 'open',
              created_at: '',
              updated_at: '',
            },
            {
              id: 43,
              workspace_id: 3,
              environment_id: 2,
              name: 'secondary-pg',
              driver: 'postgres',
              access_mode: 'open',
              created_at: '',
              updated_at: '',
            },
          ],
          page: 1,
          page_size: 100,
          total: 2,
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({
          items: [{ id: 2, workspace_id: 3, name: 'Development', created_at: '', updated_at: '' }],
          page: 1,
          page_size: 100,
          total: 1,
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({ items: [] }),
      ),
    )
  })

  afterEach(() => {
    vi.useRealTimers()
    toastMock.mockClear()
  })

  function mockFavorites(favorites: { connectionId: number | null; sqlText: string }[]) {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: favorites.map((fav, index) => ({
            id: index + 1,
            workspace_id: 3,
            account_id: 1,
            connection_id: fav.connectionId,
            name: 'Saved query',
            sql_text: fav.sqlText,
            created_at: '',
            updated_at: '',
          })),
        }),
      ),
    )
  }

  function renderPanel() {
    return {
      user: userEvent.setup(),
      ...render(
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <EditorViewRegistryContext.Provider value={views}>
              <HistoryPanel orgSlug="acme" workspace={workspace} />
            </EditorViewRegistryContext.Provider>
          </IdeStoreContext.Provider>
        </QueryClientProvider>,
      ),
    }
  }

  function mockWorkspaceHistory(fixtures: HistoryFixture[], pageSize = 25) {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/history', async ({ request }) => {
        // A small delay keeps the intermediate "fetching next page" state
        // observable in tests instead of resolving within the same tick.
        await delay(20)
        const url = new URL(request.url)
        const connectionId = url.searchParams.get('connection_id')
        const search = url.searchParams.get('q')?.toLowerCase()
        const page = Number(url.searchParams.get('page') ?? '1')
        let matching = connectionId
          ? fixtures.filter((f) => f.connectionId === Number(connectionId))
          : fixtures
        if (search) {
          matching = matching.filter((f) => f.sqlText.toLowerCase().includes(search))
        }
        const start = (page - 1) * pageSize
        const items = matching.slice(start, start + pageSize).map(entryFor)
        return HttpResponse.json({
          items,
          page,
          page_size: pageSize,
          total: matching.length,
        })
      }),
    )
  }

  it('defaults to all connections and lists entries across every connection when no tab is active', async () => {
    mockWorkspaceHistory([
      { id: 1, connectionId: 42, sqlText: 'select 1' },
      { id: 2, connectionId: 43, sqlText: 'select 2' },
    ])

    renderPanel()

    expect(await screen.findByText(sqlText('select 1'))).toBeInTheDocument()
    expect(screen.getByText(sqlText('select 2'))).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Filter by connection' })).toHaveTextContent(
      'All connections',
    )
  })

  it('defaults the filter to the active tab connection and renders history entries with a connection hint', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([
      { id: 1, connectionId: 42, sqlText: 'select 1' },
      { id: 2, connectionId: 43, sqlText: 'select 2' },
    ])

    renderPanel()

    expect(await screen.findByText(sqlText('select 1'))).toBeInTheDocument()
    expect(screen.queryByText(sqlText('select 2'))).not.toBeInTheDocument()
    expect(within(screen.getByTestId('history-row')).getByText('primary-pg')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Filter by connection' })).toHaveTextContent(
      'primary-pg',
    )
  })

  it('switches the connection filter and refetches for the selected connection', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([
      { id: 1, connectionId: 42, sqlText: 'select 1' },
      { id: 2, connectionId: 43, sqlText: 'select 2' },
    ])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))

    await user.click(screen.getByRole('combobox', { name: 'Filter by connection' }))
    await user.click(await screen.findByRole('option', { name: /secondary-pg/ }))

    expect(await screen.findByText(sqlText('select 2'))).toBeInTheDocument()
    expect(screen.queryByText(sqlText('select 1'))).not.toBeInTheDocument()
  })

  it('filters back to all connections via the pinned option', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([
      { id: 1, connectionId: 42, sqlText: 'select 1' },
      { id: 2, connectionId: 43, sqlText: 'select 2' },
    ])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))
    expect(screen.queryByText(sqlText('select 2'))).not.toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: 'Filter by connection' }))
    await user.click(await screen.findByRole('option', { name: 'All connections' }))

    expect(await screen.findByText(sqlText('select 2'))).toBeInTheDocument()
    expect(screen.getByText(sqlText('select 1'))).toBeInTheDocument()
  })

  it('marks the active tab connection with an Active hint in the filter dropdown', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))

    await user.click(screen.getByRole('combobox', { name: 'Filter by connection' }))
    const primaryOption = await screen.findByRole('option', { name: /primary-pg/ })
    expect(within(primaryOption).getByText('Active')).toBeInTheDocument()

    const secondaryOption = screen.getByRole('option', { name: /secondary-pg/ })
    expect(within(secondaryOption).queryByText('Active')).not.toBeInTheDocument()
  })

  it('fetches more history entries on scroll and shows a loading indicator', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory(
      [
        { id: 1, connectionId: 42, sqlText: 'select 1' },
        { id: 2, connectionId: 42, sqlText: 'select 2' },
      ],
      1,
    )

    renderPanel()
    await screen.findByText(sqlText('select 1'))
    expect(screen.queryByText(sqlText('select 2'))).not.toBeInTheDocument()

    const scrollEl = screen.getByTestId('history-scroll')
    Object.defineProperties(scrollEl, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 500 },
      scrollTop: { configurable: true, value: 950 },
    })
    fireEvent.scroll(scrollEl)

    expect(await screen.findByText('Loading more…')).toBeInTheDocument()
    expect(await screen.findByText(sqlText('select 2'))).toBeInTheDocument()
  })

  it('optimistically hides a deleted entry, then calls the API after the undo window elapses', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    store.getState().openTab(scratchTab)
    let deleteCalled = false
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/history', () =>
        HttpResponse.json({
          items: deleteCalled ? [] : [entryFor({ id: 1, connectionId: 42, sqlText: 'select 1' })],
          page: 1,
          page_size: 25,
          total: deleteCalled ? 0 : 1,
        }),
      ),
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/42/history/1', () => {
        deleteCalled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))
    await user.click(screen.getByRole('button', { name: 'Delete history entry' }))

    expect(screen.queryByText(sqlText('select 1'))).not.toBeInTheDocument()
    expect(deleteCalled).toBe(false)

    await vi.advanceTimersByTimeAsync(5000)
    await waitFor(() => expect(deleteCalled).toBe(true))
    vi.useRealTimers()
  })

  it('restores a deleted entry when Undo is clicked before the window elapses', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    store.getState().openTab(scratchTab)
    let deleteCalled = false
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/history', () =>
        HttpResponse.json({
          items: [entryFor({ id: 1, connectionId: 42, sqlText: 'select 1' })],
          page: 1,
          page_size: 25,
          total: 1,
        }),
      ),
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/42/history/1', () => {
        deleteCalled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))
    await user.click(screen.getByRole('button', { name: 'Delete history entry' }))
    expect(screen.queryByText(sqlText('select 1'))).not.toBeInTheDocument()

    const [, options] = toastMock.mock.calls[toastMock.mock.calls.length - 1]!
    options.action.onClick()
    expect(await screen.findByText(sqlText('select 1'))).toBeInTheDocument()

    await vi.advanceTimersByTimeAsync(5000)
    expect(deleteCalled).toBe(false)
    vi.useRealTimers()
  })

  it('copies the query to the clipboard', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))
    await user.click(screen.getByRole('button', { name: 'Copy query' }))

    expect(copyWithToastMock).toHaveBeenCalledWith('select 1', 'Query copied')
  })

  it('inserts the query at the active editor cursor', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])
    const groupId = store.getState().activeGroupId[workspace.id]!
    const editor = new EditorView({
      state: EditorState.create({ doc: 'select 2 from foo;\n' }),
    })
    editor.dispatch({ selection: { anchor: 8 } })
    views.register(`${groupId}:${scratchTab.id}`, editor)

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))
    await user.click(screen.getByRole('button', { name: 'Insert query at cursor' }))

    expect(editor.state.doc.toString()).toBe('select 2select 1 from foo;\n')
    editor.destroy()
  })

  it('filters history entries by search text', async () => {
    mockWorkspaceHistory([
      { id: 1, connectionId: 42, sqlText: 'select * from widgets' },
      { id: 2, connectionId: 43, sqlText: 'select * from gadgets' },
    ])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select * from widgets'))
    expect(screen.getByText(sqlText('select * from gadgets'))).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('Search query history…'), 'widgets')

    await waitFor(() =>
      expect(screen.queryByText(sqlText('select * from gadgets'))).not.toBeInTheDocument(),
    )
    expect(await screen.findByText(sqlText('select * from widgets'))).toBeInTheDocument()
  })

  it('shows a no-results empty state when search matches nothing', async () => {
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select * from widgets' }])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select * from widgets'))

    await user.type(screen.getByPlaceholderText('Search query history…'), 'nonexistent')

    expect(await screen.findByText('No matching queries')).toBeInTheDocument()
    expect(screen.getByText('Try a different search term.')).toBeInTheDocument()
  })

  it('saves a history entry as a favorite', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))
    await user.click(screen.getByRole('button', { name: 'Save as favorite' }))

    await user.type(screen.getByLabelText('Name'), 'Top customers')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(createFavoriteMock).toHaveBeenCalledWith({
        name: 'Top customers',
        sqlText: 'select 1',
        connectionId: 42,
      }),
    )
  })

  it('highlights the favorite button for entries already saved as a favorite', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([
      { id: 1, connectionId: 42, sqlText: 'select 1' },
      { id: 2, connectionId: 42, sqlText: 'select 2' },
    ])
    mockFavorites([{ connectionId: 42, sqlText: 'select 1' }])

    renderPanel()
    const rows = await screen.findAllByTestId('history-row')
    expect(rows).toHaveLength(2)

    const favoritedRow = within(rows[0]!).getByRole('button', {
      name: 'Remove from favorites',
    })
    const unfavoritedRow = within(rows[1]!).getByRole('button', { name: 'Save as favorite' })
    expect(favoritedRow).toBeInTheDocument()
    expect(unfavoritedRow).toBeInTheDocument()
  })

  it('expands a history row inline to show the full query, and collapses again', async () => {
    store.getState().openTab(scratchTab)
    const longSql =
      "select id, name, email from widgets where status = 'active' and archived = false and deleted = false"
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: longSql }])

    const { user } = renderPanel()
    await user.click(await screen.findByRole('button', { name: 'Expand query' }))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    const row = screen.getByTestId('history-row')
    expect(within(row).getByRole('button', { name: 'Collapse query' })).toBeInTheDocument()

    await user.click(within(row).getByRole('button', { name: 'Collapse query' }))
    expect(within(row).getByRole('button', { name: 'Expand query' })).toBeInTheDocument()
  })

  it('does not allow expanding a short single-line query', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])

    renderPanel()
    await screen.findByText(sqlText('select 1'))
    const row = screen.getByTestId('history-row')

    expect(within(row).queryByRole('button', { name: 'Expand query' })).not.toBeInTheDocument()
    expect(within(row).queryByRole('button', { name: 'select 1' })).not.toBeInTheDocument()
  })

  it('puts the raw query text on the drag payload when a row is dragged', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select   1\n' }])

    renderPanel()
    const row = await screen.findByTestId('history-row')

    const dataTransfer = {
      setData: vi.fn(),
      effectAllowed: '',
    } as unknown as DataTransfer
    fireEvent.dragStart(row, { dataTransfer })

    expect(dataTransfer.setData).toHaveBeenCalledWith('text/plain', 'select   1\n')
    expect(dataTransfer.effectAllowed).toBe('copy')
  })

  it('removes a favorite when its already-favorited button is clicked', async () => {
    store.getState().openTab(scratchTab)
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])
    mockFavorites([{ connectionId: 42, sqlText: 'select 1' }])

    const { user } = renderPanel()
    const row = await screen.findByTestId('history-row')

    await user.click(within(row).getByRole('button', { name: 'Remove from favorites' }))

    await waitFor(() => expect(removeFavoriteMock).toHaveBeenCalledWith(1))
  })

  it('disables Clear All until a specific connection is selected', async () => {
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])

    renderPanel()
    await screen.findByText(sqlText('select 1'))

    expect(screen.getByRole('button', { name: 'Clear all history' })).toBeDisabled()
  })

  it('clears history for the selected connection after confirming', async () => {
    store.getState().openTab(scratchTab)
    let cleared = false
    mockWorkspaceHistory([{ id: 1, connectionId: 42, sqlText: 'select 1' }])
    server.use(
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/42/history', () => {
        cleared = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const { user } = renderPanel()
    await screen.findByText(sqlText('select 1'))

    const clearButton = screen.getByRole('button', { name: 'Clear all history' })
    expect(clearButton).toBeEnabled()
    await user.click(clearButton)

    await user.click(await screen.findByRole('button', { name: 'Clear all' }))

    await waitFor(() => expect(cleared).toBe(true))
  })
})
