import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ObjectRef } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { SchemaTree } from './SchemaTree'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const ref: ObjectRef = { namespace: 'public', kind: 'table', name: 'orders' }

describe('SchemaTree', () => {
  let store: ReturnType<typeof createIdeStore>
  let editorViews: ReturnType<typeof createEditorViewRegistry>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    editorViews = createEditorViewRegistry()
  })

  function renderTree(filter = '', onConnect = vi.fn()) {
    return render(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <EditorViewRegistryContext.Provider value={editorViews}>
            <SchemaTree
              orgSlug="acme"
              workspaceId={3}
              connectionId={7}
              driver="postgres"
              filter={filter}
              onConnect={onConnect}
            />
          </EditorViewRegistryContext.Provider>
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  function respondReady() {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/catalog', () =>
        HttpResponse.json({
          catalog: {
            connection: 'warehouse',
            dialect: 'postgres',
            database: 'analytics',
            generated_at: '',
            namespaces: [{ name: 'public', groups: [{ kind: 'table', objects: [ref] }] }],
          },
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({
          spec: {
            dialect: 'postgres',
            kinds: [
              {
                kind: 'table',
                label: 'Table',
                plural_label: 'Tables',
                order: 1,
                relational: true,
                supports_diagram: true,
                listing: 'enumerated',
              },
            ],
          },
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects', () =>
        HttpResponse.json({
          objects: [
            {
              ref,
              relational: {
                columns: [{ name: 'id', data_type: 'bigint', nullable: false, ordinal: 1 }],
                primary_key: ['id'],
                foreign_keys: [],
                indexes: [],
              },
            },
          ],
        }),
      ),
    )
  }

  it('offers connection recovery when ephemeral schema access requires a session', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/catalog', () =>
        HttpResponse.json(
          { error: { code: 'bad_request', message: 'X-Warden-Session header is required.' } },
          { status: 400 },
        ),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({ spec: { dialect: 'postgres', kinds: [] } }),
      ),
    )
    const onConnect = vi.fn()
    renderTree('', onConnect)
    expect(await screen.findByText('Not connected.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))
    expect(onConnect).toHaveBeenCalledOnce()
  })

  it('loads a persisted schema snapshot without a live session', async () => {
    respondReady()
    renderTree()
    fireEvent.click(await screen.findByText('Tables'))
    expect(await screen.findByText('orders')).toBeInTheDocument()
  })

  it('distinguishes unsupported inspection from a generic failure', async () => {
    store.getState().setSession(7, 'session-7')
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/catalog', () =>
        HttpResponse.json(
          {
            error: { code: 'not_implemented', message: 'Unsupported' },
          },
          { status: 501 },
        ),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({ spec: { dialect: 'postgres', kinds: [] } }),
      ),
    )
    renderTree()
    expect(
      await screen.findByText("This driver doesn't support schema inspection."),
    ).toBeInTheDocument()
  })

  it('loads object details lazily and opens the object context on double click', async () => {
    store.getState().setSession(7, 'session-7')
    respondReady()
    renderTree()

    fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
    const objectRow = await screen.findByRole('button', { name: 'orders' })
    expect(screen.queryByText('bigint')).not.toBeInTheDocument()
    fireEvent.click(objectRow)
    expect(await screen.findByText('bigint')).toBeInTheDocument()

    fireEvent.doubleClick(objectRow)
    await waitFor(() =>
      expect(store.getState().tabs).toEqual(
        expect.arrayContaining([expect.objectContaining({ kind: 'object', objectRef: ref })]),
      ),
    )
  })

  it('force-opens matching branches and reports an empty search', async () => {
    store.getState().setSession(7, 'session-7')
    respondReady()
    const view = renderTree('orders')
    expect(await screen.findByRole('button', { name: 'orders' })).toBeInTheDocument()

    view.rerender(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <EditorViewRegistryContext.Provider value={editorViews}>
            <SchemaTree
              orgSlug="acme"
              workspaceId={3}
              connectionId={7}
              driver="postgres"
              filter="missing"
            />
          </EditorViewRegistryContext.Provider>
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText('No matches.')).toBeInTheDocument()
  })
})
