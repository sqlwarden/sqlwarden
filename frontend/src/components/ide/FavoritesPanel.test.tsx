import { QueryClientProvider } from '@tanstack/react-query'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { organizationRuntimeSettingsFixture } from '#/test/fixtures'
import { server } from '#/test/server'
import { FavoritesPanel } from './FavoritesPanel'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { createIdeStore, IdeStoreContext, type EditorTab } from './useIdeStore'

const { copyWithToastMock, toastMock } = vi.hoisted(() => ({
  copyWithToastMock: vi.fn(),
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
      measureElement: vi.fn(),
    }
  },
}))

vi.mock('./contextMenus/clipboard', () => ({ copyWithToast: copyWithToastMock }))

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

type FavoriteFixture = {
  id: number
  name: string
  sqlText: string
}

function sqlText(value: string) {
  return (_content: string, element: Element | null) => {
    if (!element || element.textContent !== value) return false
    return Array.from(element.children).every((child) => child.textContent !== value)
  }
}

function favoriteFor(fixture: FavoriteFixture) {
  return {
    id: fixture.id,
    workspace_id: 3,
    account_id: 1,
    connection_id: 42,
    name: fixture.name,
    sql_text: fixture.sqlText,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }
}

function mockFavorites(fixtures: FavoriteFixture[]) {
  server.use(
    http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', ({ request }) => {
      const search = new URL(request.url).searchParams.get('q')?.toLowerCase()
      const matching = search
        ? fixtures.filter(
            (f) =>
              f.name.toLowerCase().includes(search) || f.sqlText.toLowerCase().includes(search),
          )
        : fixtures
      return HttpResponse.json({ items: matching.map(favoriteFor) })
    }),
  )
}

describe('FavoritesPanel', () => {
  let store: ReturnType<typeof createIdeStore>
  let views: ReturnType<typeof createEditorViewRegistry>

  beforeEach(() => {
    copyWithToastMock.mockClear()
    toastMock.mockClear()
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
          ],
          page: 1,
          page_size: 100,
          total: 1,
        }),
      ),
    )
  })

  function renderPanel() {
    return {
      user: userEvent.setup(),
      ...render(
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <EditorViewRegistryContext.Provider value={views}>
              <FavoritesPanel orgSlug="acme" workspace={workspace} />
            </EditorViewRegistryContext.Provider>
          </IdeStoreContext.Provider>
        </QueryClientProvider>,
      ),
    }
  }

  it('shows an empty state with no favorites', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({ items: [] }),
      ),
    )

    renderPanel()

    expect(await screen.findByText('No saved favorites yet')).toBeInTheDocument()
  })

  it('renders favorites from the backend', async () => {
    mockFavorites([{ id: 1, name: 'Top customers', sqlText: 'select 1' }])

    renderPanel()

    expect(await screen.findByText('Top customers')).toBeInTheDocument()
    expect(screen.getByText(sqlText('select 1'))).toBeInTheDocument()
  })

  it('optimistically hides a deleted favorite, then calls the API after the undo window elapses', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    let deleted = false
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: deleted
            ? []
            : [favoriteFor({ id: 1, name: 'Top customers', sqlText: 'select 1' })],
        }),
      ),
      http.delete('/api/v1/orgs/acme/workspaces/3/query-favorites/1', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    await user.click(screen.getByRole('button', { name: 'Delete favorite' }))

    expect(screen.queryByText('Top customers')).not.toBeInTheDocument()
    expect(deleted).toBe(false)

    await vi.advanceTimersByTimeAsync(5000)
    await waitFor(() => expect(deleted).toBe(true))
    vi.useRealTimers()
  })

  it('restores a deleted favorite when Undo is clicked before the window elapses', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    let deleted = false
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: [favoriteFor({ id: 1, name: 'Top customers', sqlText: 'select 1' })],
        }),
      ),
      http.delete('/api/v1/orgs/acme/workspaces/3/query-favorites/1', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    await user.click(screen.getByRole('button', { name: 'Delete favorite' }))
    expect(screen.queryByText('Top customers')).not.toBeInTheDocument()

    const [, options] = toastMock.mock.calls[toastMock.mock.calls.length - 1]!
    options.action.onClick()
    expect(await screen.findByText('Top customers')).toBeInTheDocument()

    await vi.advanceTimersByTimeAsync(5000)
    expect(deleted).toBe(false)
    vi.useRealTimers()
  })

  it('puts the raw query text on the drag payload when a row is dragged', async () => {
    mockFavorites([{ id: 1, name: 'Top customers', sqlText: 'select   1\n' }])

    renderPanel()
    const row = await screen.findByTestId('favorite-row')

    const dataTransfer = {
      setData: vi.fn(),
      effectAllowed: '',
    } as unknown as DataTransfer
    fireEvent.dragStart(row, { dataTransfer })

    expect(dataTransfer.setData).toHaveBeenCalledWith('text/plain', 'select   1\n')
    expect(dataTransfer.effectAllowed).toBe('copy')
  })

  it('copies the favorite SQL to the clipboard', async () => {
    mockFavorites([{ id: 1, name: 'Top customers', sqlText: 'select 1' }])

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    await user.click(screen.getByRole('button', { name: 'Copy query' }))

    expect(copyWithToastMock).toHaveBeenCalledWith('select 1', 'Query copied')
  })

  it('inserts the favorite SQL at the active editor cursor', async () => {
    store.getState().openTab(scratchTab)
    mockFavorites([{ id: 1, name: 'Top customers', sqlText: 'select 1' }])
    const groupId = store.getState().activeGroupId[workspace.id]!
    const editor = new EditorView({
      state: EditorState.create({ doc: 'select 2 from foo;\n' }),
    })
    editor.dispatch({ selection: { anchor: 8 } })
    views.register(`${groupId}:${scratchTab.id}`, editor)

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    await user.click(screen.getByRole('button', { name: 'Insert query at cursor' }))

    expect(editor.state.doc.toString()).toBe('select 2select 1 from foo;\n')
    editor.destroy()
  })

  it('filters favorites by search text', async () => {
    mockFavorites([
      { id: 1, name: 'Top customers', sqlText: 'select * from widgets' },
      { id: 2, name: 'Top orders', sqlText: 'select * from gadgets' },
    ])

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    expect(screen.getByText('Top orders')).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('Search favorites…'), 'widgets')

    await waitFor(() => expect(screen.queryByText('Top orders')).not.toBeInTheDocument())
    expect(await screen.findByText('Top customers')).toBeInTheDocument()
  })

  it('expands a favorite row inline to show the full query', async () => {
    mockFavorites([
      {
        id: 1,
        name: 'Top customers',
        sqlText:
          'select * from widgets where widgets.id in (select widget_id from orders) order by widgets.name',
      },
    ])

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    await user.click(await screen.findByRole('button', { name: 'Expand query' }))

    expect(await screen.findByRole('button', { name: 'Collapse query' })).toBeInTheDocument()
  })

  it('shows a no-results empty state when search matches nothing', async () => {
    mockFavorites([{ id: 1, name: 'Top customers', sqlText: 'select 1' }])

    const { user } = renderPanel()
    await screen.findByText('Top customers')

    await user.type(screen.getByPlaceholderText('Search favorites…'), 'nonexistent')

    expect(await screen.findByText('No matching favorites')).toBeInTheDocument()
    expect(screen.getByText('Try a different search term.')).toBeInTheDocument()
  })
})
