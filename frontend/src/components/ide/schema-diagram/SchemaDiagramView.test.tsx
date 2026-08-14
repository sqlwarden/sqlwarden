import { QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { get } from 'idb-keyval'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext, type EditorTab } from '../useIdeStore'
import { SchemaDiagramView } from './SchemaDiagramView'

vi.mock('idb-keyval', () => ({
  del: vi.fn(() => Promise.resolve()),
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
}))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 1,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}

const tab: EditorTab = {
  id: 'diagram:7:scope:public',
  workspaceId: 3,
  title: 'public diagram',
  kind: 'diagram',
  content: '',
  connectionId: 7,
  driver: 'postgres',
  diagramTarget: { kind: 'scope', scope: [{ kind: 'schema', name: 'public' }] },
}

describe('SchemaDiagramView', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    vi.mocked(get).mockResolvedValue(null)
    store = createIdeStore('acme', 1, 'ephemeral')
  })

  function renderDiagram(editorTab: EditorTab = tab) {
    return {
      user: userEvent.setup(),
      ...render(
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <SchemaDiagramView orgSlug="acme" workspace={workspace} tab={editorTab} />
          </IdeStoreContext.Provider>
        </QueryClientProvider>,
      ),
    }
  }

  function schemaHandlers(options: { supportsDiagram?: boolean; forbidden?: boolean } = {}) {
    const base = '/api/v1/orgs/acme/workspaces/3/connections/7/schema'
    server.use(
      http.get(`${base}/spec`, () =>
        options.forbidden
          ? HttpResponse.json({ error: { message: 'Forbidden' } }, { status: 403 })
          : HttpResponse.json({
              spec: {
                dialect: 'postgres',
                kinds:
                  options.supportsDiagram === false
                    ? []
                    : [
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
      http.get(`${base}/directory`, () =>
        HttpResponse.json({
          directory: {
            generated_at: '',
            roots: [
              {
                segment: { kind: 'schema', name: 'public' },
                path: [{ kind: 'schema', name: 'public' }],
                groups: [],
              },
            ],
          },
        }),
      ),
      http.get(`${base}/relationships`, () =>
        HttpResponse.json({
          graph: { scope: [{ kind: 'schema', name: 'public' }], relationships: [] },
        }),
      ),
    )
  }

  it('explains a malformed diagram tab without issuing schema requests', () => {
    renderDiagram({ ...tab, connectionId: undefined, diagramTarget: undefined })
    expect(screen.getByText('This tab is missing its diagram target.')).toBeInTheDocument()
  })

  it('reconnects a stale diagram tab and stores the new session', async () => {
    schemaHandlers({ supportsDiagram: false })
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/connect', () =>
        HttpResponse.json({ session_id: 'session-new' }),
      ),
    )
    const { user } = renderDiagram()

    expect(screen.getByText('postgres · connection not available')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Reconnect' }))

    await waitFor(() => expect(store.getState().sessions[7]).toBe('session-new'))
    expect(
      await screen.findByText("Diagrams aren't available for this connection."),
    ).toBeInTheDocument()
    expect(store.getState().connectionStatus[7]).toBeUndefined()
  })

  it('shows unsupported, forbidden, and empty schema states', async () => {
    store.getState().setSession(7, 'session-7')
    schemaHandlers({ supportsDiagram: false })
    const unsupported = renderDiagram()
    expect(
      await screen.findByText("Diagrams aren't available for this connection."),
    ).toBeInTheDocument()
    unsupported.unmount()

    schemaHandlers({ forbidden: true })
    const forbidden = renderDiagram()
    expect(
      await screen.findByText('You no longer have access to this connection.'),
    ).toBeInTheDocument()
    forbidden.unmount()

    schemaHandlers()
    renderDiagram()
    expect(await screen.findByText('No tables to diagram in this schema.')).toBeInTheDocument()
  })

  it('refreshes the backing schema through the backend endpoint', async () => {
    store.getState().setSession(7, 'session-7')
    schemaHandlers()
    const scope = [{ kind: 'schema', name: 'public' }]
    const customer = { scope, kind: 'table', name: 'customer' }
    let refreshes = 0
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            generated_at: '',
            roots: [
              {
                segment: scope[0],
                path: scope,
                groups: [{ kind: 'table', objects: [customer] }],
              },
            ],
          },
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects', () =>
        HttpResponse.json({
          objects: [
            {
              ref: customer,
              relational: { columns: [], primary_key: [], foreign_keys: [], indexes: [] },
            },
          ],
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () => {
        refreshes++
        return HttpResponse.json({
          status: 'ok',
          mode: 'persistent',
          snapshot_id: 'snapshot-2',
          generated_at: '2026-08-06T00:00:00Z',
        })
      }),
    )
    const { user } = renderDiagram()

    await user.click(await screen.findByRole('button', { name: 'Refresh schema' }))

    await waitFor(() => expect(refreshes).toBe(1))
  })

  it('reconciles a renamed column after refreshing the schema', async () => {
    store.getState().setSession(7, 'session-7')
    schemaHandlers()
    const scope = [{ kind: 'schema', name: 'public' }]
    const customer = { scope, kind: 'table', name: 'customer' }
    let refreshes = 0
    let renamed = false
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            generated_at: '',
            roots: [
              {
                segment: scope[0],
                path: scope,
                groups: [{ kind: 'table', objects: [customer] }],
              },
            ],
          },
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects', () =>
        HttpResponse.json({
          objects: [
            {
              ref: customer,
              relational: {
                columns: [
                  {
                    name: renamed ? 'full_name' : 'name',
                    data_type: 'text',
                    nullable: false,
                    ordinal: 1,
                  },
                ],
                primary_key: [],
                foreign_keys: [],
                indexes: [],
              },
            },
          ],
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () => {
        refreshes++
        renamed = true
        return HttpResponse.json({
          status: 'ok',
          mode: 'persistent',
          snapshot_id: 'snapshot-2',
          generated_at: '2026-08-06T00:00:00Z',
        })
      }),
    )
    const { user } = renderDiagram()

    expect(await screen.findByText('name')).toBeInTheDocument()
    expect(screen.queryByText('full_name')).not.toBeInTheDocument()

    await user.click(await screen.findByRole('button', { name: 'Refresh schema' }))

    await waitFor(() => expect(refreshes).toBe(1))
    expect(await screen.findByText('full_name')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('name')).not.toBeInTheDocument())
  })

  it('restores a persisted relationship diagram without attaching missing handles', async () => {
    const scope = [{ kind: 'schema', name: 'public' }]
    const customer = { scope, kind: 'table', name: 'customer' }
    const storeRef = { scope, kind: 'table', name: 'store' }
    const reactFlowErrors: unknown[][] = []
    const errorSpy = vi.spyOn(console, 'error').mockImplementation((...args) => {
      if (args.some((arg) => String(arg).includes("Couldn't create edge")))
        reactFlowErrors.push(args)
    })
    store.getState().setSession(7, 'session-7')
    vi.mocked(get).mockResolvedValueOnce(
      JSON.stringify({
        present: ['schema=public table customer', 'schema=public table store'],
        positions: {
          'schema=public table customer': { x: 0, y: 0 },
          'schema=public table store': { x: 300, y: 0 },
        },
        collapsed: [],
        keysOnly: true,
      }),
    )
    const base = '/api/v1/orgs/acme/workspaces/3/connections/7/schema'
    server.use(
      http.get(`${base}/spec`, () =>
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
      http.get(`${base}/directory`, () =>
        HttpResponse.json({
          directory: {
            connection: 'test',
            dialect: 'postgres',
            database: 'test',
            generated_at: '',
            roots: [
              {
                segment: scope[0],
                path: scope,
                groups: [{ kind: 'table', objects: [customer, storeRef] }],
              },
            ],
          },
        }),
      ),
      http.get(`${base}/relationships`, () =>
        HttpResponse.json({
          graph: {
            scope,
            relationships: [
              {
                name: 'fk_customer_store',
                source: customer,
                columns: ['store_id'],
                references: storeRef,
                referenced_columns: ['id'],
              },
            ],
          },
        }),
      ),
      http.post(`${base}/objects`, async ({ request }) => {
        const body = (await request.json()) as { refs: Array<{ name: string }> }
        const ref = body.refs[0]
        const isCustomer = ref.name === 'customer'
        return HttpResponse.json({
          objects: [
            {
              ref: isCustomer ? customer : storeRef,
              relational: {
                columns: isCustomer
                  ? [{ name: 'store_id', data_type: 'integer', nullable: false, ordinal: 1 }]
                  : [{ name: 'id', data_type: 'integer', nullable: false, ordinal: 1 }],
                primary_key: isCustomer ? [] : ['id'],
                foreign_keys: isCustomer
                  ? [
                      {
                        name: 'fk_customer_store',
                        columns: ['store_id'],
                        references: storeRef,
                        referenced_columns: ['id'],
                      },
                    ]
                  : [],
                indexes: [],
              },
            },
          ],
        })
      }),
    )

    try {
      renderDiagram()
      expect(await screen.findByRole('status')).toHaveTextContent('Preparing diagram')
      expect(await screen.findByText('store_id')).toBeInTheDocument()
      await act(async () => {
        await new Promise<void>((resolve) =>
          window.requestAnimationFrame(() =>
            window.requestAnimationFrame(() => window.requestAnimationFrame(() => resolve())),
          ),
        )
      })
      await waitFor(() => expect(screen.queryByRole('status')).not.toBeInTheDocument())
      expect(reactFlowErrors).toEqual([])
    } finally {
      errorSpy.mockRestore()
    }
  })
})
