import { QueryClientProvider } from '@tanstack/react-query'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { render, screen, waitFor, within } from '@testing-library/react'
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

const { copyWithToastMock } = vi.hoisted(() => ({
  copyWithToastMock: vi.fn(),
}))

vi.mock('./object-detail/ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => <pre>{value}</pre>,
}))

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
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: [
            {
              id: 1,
              workspace_id: 3,
              account_id: 1,
              connection_id: 42,
              name: 'Top customers',
              sql_text: 'select 1',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
        }),
      ),
    )

    renderPanel()

    expect(await screen.findByText('Top customers')).toBeInTheDocument()
    expect(screen.getByText('select 1')).toBeInTheDocument()
  })

  it('deletes a favorite and refetches the list', async () => {
    let deleted = false
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: deleted
            ? []
            : [
                {
                  id: 1,
                  workspace_id: 3,
                  account_id: 1,
                  connection_id: 42,
                  name: 'Top customers',
                  sql_text: 'select 1',
                  created_at: new Date().toISOString(),
                  updated_at: new Date().toISOString(),
                },
              ],
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

    await waitFor(() => expect(screen.queryByText('Top customers')).not.toBeInTheDocument())
  })

  it('copies the favorite SQL to the clipboard', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: [
            {
              id: 1,
              workspace_id: 3,
              account_id: 1,
              connection_id: 42,
              name: 'Top customers',
              sql_text: 'select 1',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
        }),
      ),
    )

    const { user } = renderPanel()
    await screen.findByText('Top customers')
    await user.click(screen.getByRole('button', { name: 'Copy query' }))

    expect(copyWithToastMock).toHaveBeenCalledWith('select 1', 'Query copied')
  })

  it('inserts the favorite SQL at the active editor cursor', async () => {
    store.getState().openTab(scratchTab)
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: [
            {
              id: 1,
              workspace_id: 3,
              account_id: 1,
              connection_id: 42,
              name: 'Top customers',
              sql_text: 'select 1',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
        }),
      ),
    )
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

  it('opens a dialog with the full query and actions when the query is clicked', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/query-favorites', () =>
        HttpResponse.json({
          items: [
            {
              id: 1,
              workspace_id: 3,
              account_id: 1,
              connection_id: 42,
              name: 'Top customers',
              sql_text: 'select 1 from widgets',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
        }),
      ),
    )

    const { user } = renderPanel()
    await user.click(await screen.findByRole('button', { name: 'select 1 from widgets' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Top customers')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Copy query' })).toBeInTheDocument()
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
