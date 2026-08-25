import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ObjectRef, SchemaEditSpec } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { SchemaTree } from './SchemaTree'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { ContextMenuProvider } from '#/components/ui/context-menu'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

vi.mock('./object-detail/ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => (
    <pre data-testid="generated-sql-preview">{value}</pre>
  ),
}))

const scope = [{ kind: 'schema', name: 'public' }]
const ref: ObjectRef = { scope, kind: 'table', name: 'orders' }

describe('SchemaTree', () => {
  let store: ReturnType<typeof createIdeStore>
  let editorViews: ReturnType<typeof createEditorViewRegistry>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    editorViews = createEditorViewRegistry()
    mockPermissions([])
  })

  function renderTree(filter = '', onConnect = vi.fn()) {
    return render(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <EditorViewRegistryContext.Provider value={editorViews}>
            <ContextMenuProvider>
              <SchemaTree
                orgSlug="acme"
                workspaceId={3}
                connectionId={7}
                driver="postgres"
                filter={filter}
                onConnect={onConnect}
              />
            </ContextMenuProvider>
          </EditorViewRegistryContext.Provider>
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  function respondReady() {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            connection: 'warehouse',
            dialect: 'postgres',
            database: 'analytics',
            generated_at: '',
            roots: [
              {
                segment: scope[0],
                path: scope,
                groups: [{ kind: 'table', objects: [ref] }],
              },
            ],
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

  const createTableEditor: SchemaEditSpec = {
    operations: ['create_table'],
    column_types: ['text', 'integer'],
    creatable_table_scope_kinds: ['schema'],
    droppable_object_kinds: [],
    droppable_scope_kinds: [],
    supports_cascade: false,
  }

  const dropEditor: SchemaEditSpec = {
    operations: ['drop_object', 'drop_column', 'drop_index'],
    column_types: [],
    creatable_table_scope_kinds: [],
    droppable_object_kinds: ['table'],
    droppable_scope_kinds: [],
    supports_cascade: true,
  }

  const tableSpec = {
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
  }

  function mockPermissions(permissions: string[]) {
    server.use(
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({ resource_type: 'connection', resource_id: 7, permissions }),
      ),
    )
  }

  it('offers connection recovery when ephemeral schema access requires a session', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
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

  it('renders an empty state when an empty snapshot contains null roots', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            connection: 'warehouse',
            engine: 'postgres',
            default_scope: scope,
            generated_at: '',
            roots: null,
          },
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({ spec: { dialect: 'postgres', kinds: [] } }),
      ),
    )

    renderTree()
    expect(await screen.findByText('No objects.')).toBeInTheDocument()
  })

  it('renders an empty state for a PostgreSQL database scope with no schemas or objects', async () => {
    const databaseScope = [{ kind: 'database', name: 'postgres' }]
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            connection: 'warehouse',
            engine: 'postgres',
            default_scope: databaseScope,
            generated_at: '',
            roots: [{ path: databaseScope, groups: [] }],
          },
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({ spec: { dialect: 'postgres', kinds: [] } }),
      ),
    )

    renderTree()
    expect(await screen.findByText('No objects.')).toBeInTheDocument()
  })

  it('renders each empty schema under an empty PostgreSQL database so its own create table stays reachable', async () => {
    store.getState().setSession(7, 'session-7')
    const databaseScope = [{ kind: 'database', name: 'postgres' }]
    const publicScope = [...databaseScope, { kind: 'schema', name: 'public' }]
    const reportingScope = [...databaseScope, { kind: 'schema', name: 'reporting' }]
    let mutationBody: unknown
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            connection: 'warehouse',
            engine: 'postgres',
            default_scope: databaseScope,
            generated_at: '',
            roots: [
              {
                path: databaseScope,
                groups: [],
                children: [
                  { path: publicScope, groups: [] },
                  { path: reportingScope, groups: [] },
                ],
              },
            ],
          },
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({ spec: tableSpec, editor: createTableEditor }),
      ),
      http.post(
        '/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations',
        async ({ request }) => {
          mutationBody = await request.json()
          return HttpResponse.json({
            applied: true,
            schema: { status: 'available', mode: 'ephemeral' },
            transaction: { mode: 'auto', open: false, pending_statements: 0 },
          })
        },
      ),
    )
    mockPermissions(['conn:ddl'])

    renderTree()

    expect(screen.queryByText('No objects.')).not.toBeInTheDocument()
    const publicRow = await screen.findByRole('button', { name: 'public' })
    const reportingRow = await screen.findByRole('button', { name: 'reporting' })
    expect(publicRow).toBeInTheDocument()
    expect(reportingRow).toBeInTheDocument()

    fireEvent.contextMenu(reportingRow)
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Create table…' }))
    fireEvent.change(await screen.findByLabelText('Table name'), { target: { value: 'events' } })
    fireEvent.change(screen.getByLabelText('Column name'), { target: { value: 'id' } })
    fireEvent.click(screen.getByRole('button', { name: /^create table$/i }))

    await waitFor(() =>
      expect(mutationBody).toEqual({
        operation: 'create_table',
        scope: reportingScope,
        name: 'events',
        columns: [{ name: 'id', data_type: 'text', nullable: true, primary_key: false }],
      }),
    )
  })

  it('automatically reloads a directory while its snapshot is being prepared', async () => {
    let directoryRequests = 0
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () => {
        directoryRequests++
        if (directoryRequests === 1) {
          return HttpResponse.json({ status: 'pending' }, { status: 202 })
        }
        return HttpResponse.json({
          status: 'ready',
          directory: {
            connection: 'warehouse',
            dialect: 'postgres',
            database: 'analytics',
            generated_at: '',
            roots: [
              {
                segment: scope[0],
                path: scope,
                groups: [{ kind: 'table', objects: [ref] }],
              },
            ],
          },
        })
      }),
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
    )

    renderTree()

    expect(await screen.findByText('Preparing schema snapshot…')).toBeInTheDocument()
    expect(await screen.findByText('Tables', {}, { timeout: 2_500 })).toBeInTheDocument()
    expect(directoryRequests).toBe(2)
  })

  it('distinguishes unsupported inspection from a generic failure', async () => {
    store.getState().setSession(7, 'session-7')
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
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

  it('keeps routine and sequence rows compact while condensing trigger details', async () => {
    store.getState().setSession(7, 'session-7')
    const refs: ObjectRef[] = [
      { scope, kind: 'function', name: 'calculate_tax' },
      { scope, kind: 'procedure', name: 'refresh_totals' },
      { scope, kind: 'sequence', name: 'orders_id_seq' },
      { scope, kind: 'trigger', name: 'orders_audit' },
    ]
    const requestedKinds: string[] = []
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            connection: 'warehouse',
            engine: 'postgres',
            default_scope: scope,
            generated_at: '',
            roots: [
              {
                path: scope,
                groups: refs.map((objectRef) => ({
                  kind: objectRef.kind,
                  objects: [objectRef],
                })),
              },
            ],
          },
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({
          spec: {
            dialect: 'postgres',
            kinds: refs.map((objectRef, order) => ({
              kind: objectRef.kind,
              label: objectRef.kind[0].toUpperCase() + objectRef.kind.slice(1),
              plural_label: objectRef.kind[0].toUpperCase() + objectRef.kind.slice(1) + 's',
              order,
              relational: false,
              supports_diagram: false,
              listing: 'enumerated',
            })),
          },
        }),
      ),
      http.post(
        '/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects',
        async ({ request }) => {
          const body = (await request.json()) as { refs: ObjectRef[] }
          const objectRef = body.refs[0]
          requestedKinds.push(objectRef.kind)
          if (objectRef.kind === 'sequence') {
            return HttpResponse.json({
              objects: [
                {
                  ref: objectRef,
                  descriptors: [
                    {
                      kind: 'fields',
                      title: 'Sequence',
                      fields: [{ name: 'Data type', value: 'bigint' }],
                    },
                  ],
                },
              ],
            })
          }
          return HttpResponse.json({
            objects: [
              {
                ref: objectRef,
                descriptors: [
                  {
                    kind: 'fields',
                    title: 'Trigger',
                    fields: [
                      { name: 'Timing', value: 'BEFORE' },
                      { name: 'Event', value: 'INSERT' },
                      { name: 'Table', value: 'public.orders' },
                    ],
                  },
                  {
                    kind: 'source',
                    title: 'Statement',
                    source: { language: 'sql', body: 'BEGIN audit(); END' },
                  },
                ],
              },
            ],
          })
        },
      ),
    )

    renderTree()
    for (const label of ['Functions', 'Procedures', 'Sequences', 'Triggers']) {
      fireEvent.click(await screen.findByRole('button', { name: new RegExp(label) }))
    }

    const functionRow = screen.getByRole('button', { name: 'calculate_tax' })
    const procedureRow = screen.getByRole('button', { name: 'refresh_totals' })
    const sequenceRow = screen.getByRole('button', { name: /orders_id_seq/ })
    expect(functionRow).not.toHaveAttribute('aria-expanded')
    expect(procedureRow).not.toHaveAttribute('aria-expanded')
    expect(sequenceRow).not.toHaveAttribute('aria-expanded')
    expect(await screen.findByText('bigint')).toBeInTheDocument()
    expect(requestedKinds).toEqual(['sequence'])

    fireEvent.doubleClick(functionRow)
    await waitFor(() =>
      expect(store.getState().tabs).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            kind: 'object',
            objectRef: expect.objectContaining({ kind: 'function', name: 'calculate_tax' }),
          }),
        ]),
      ),
    )

    const triggerRow = screen.getByRole('button', { name: 'orders_audit' })
    expect(triggerRow).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(triggerRow)
    expect(await screen.findByText('BEFORE')).toBeInTheDocument()
    expect(screen.getByText('INSERT')).toBeInTheDocument()
    expect(screen.getByText('public.orders')).toBeInTheDocument()
    expect(screen.queryByText('BEGIN audit(); END')).not.toBeInTheDocument()
    expect(requestedKinds).toEqual(['sequence', 'trigger'])
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

  it('uses the backend refresh endpoint from schema group menus', async () => {
    store.getState().setSession(7, 'session-7')
    respondReady()
    let refreshes = 0
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () => {
        refreshes++
        return HttpResponse.json({ status: 'ok', mode: 'ephemeral' })
      }),
    )
    renderTree()

    fireEvent.contextMenu(await screen.findByRole('button', { name: /Tables/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Refresh' }))
    await waitFor(() => expect(refreshes).toBe(1))
  })

  it('opens a scope diagram from a diagram-capable object group menu', async () => {
    store.getState().setSession(7, 'session-7')
    respondReady()
    renderTree()

    fireEvent.contextMenu(await screen.findByRole('button', { name: /Tables/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'View diagram' }))

    await waitFor(() =>
      expect(store.getState().tabs).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            kind: 'diagram',
            diagramTarget: { kind: 'scope', scope },
          }),
        ]),
      ),
    )
  })

  it('omits the group diagram action when the object kind does not support diagrams', async () => {
    store.getState().setSession(7, 'session-7')
    respondReady()
    server.use(
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
                supports_diagram: false,
                listing: 'enumerated',
              },
            ],
          },
        }),
      ),
    )
    renderTree()

    fireEvent.contextMenu(await screen.findByRole('button', { name: /Tables/ }))
    expect(screen.queryByRole('menuitem', { name: 'View diagram' })).not.toBeInTheDocument()
  })

  it('omits a creation entry for non-table object groups; keeps Create table gated for Tables', async () => {
    store.getState().setSession(7, 'session-7')
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
        HttpResponse.json({
          directory: {
            connection: 'warehouse',
            dialect: 'postgres',
            database: 'analytics',
            generated_at: '',
            roots: [
              {
                segment: scope[0],
                path: scope,
                groups: [
                  { kind: 'table', objects: [ref] },
                  { kind: 'view', objects: [] },
                ],
              },
            ],
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
              {
                kind: 'view',
                label: 'View',
                plural_label: 'Views',
                order: 2,
                relational: true,
                supports_diagram: false,
                listing: 'enumerated',
              },
            ],
          },
          editor: createTableEditor,
        }),
      ),
    )
    mockPermissions([])

    renderTree()

    fireEvent.contextMenu(await screen.findByRole('button', { name: /Views/ }))
    expect(screen.queryByRole('menuitem', { name: /^New/ })).not.toBeInTheDocument()
    expect(screen.queryByText('soon')).not.toBeInTheDocument()
    expect(await screen.findByRole('menuitem', { name: 'Refresh' })).toBeInTheDocument()

    fireEvent.contextMenu(await screen.findByRole('button', { name: /Tables/ }))
    const createItem = await screen.findByRole('menuitem', { name: /^New Table/ })
    expect(createItem).toHaveAttribute('data-disabled')
    expect(
      screen.getByText('You do not have permission to change this schema.'),
    ).toBeInTheDocument()
  })

  describe('schema editing', () => {
    it('lets a permitted, connected user create the first table from an empty scope, then refetches', async () => {
      store.getState().setSession(7, 'session-7')
      let directoryRequests = 0
      let mutationBody: unknown
      server.use(
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () => {
          directoryRequests++
          return HttpResponse.json({
            directory: {
              connection: 'warehouse',
              dialect: 'postgres',
              database: 'analytics',
              generated_at: '',
              roots:
                directoryRequests === 1
                  ? [{ path: scope, groups: [] }]
                  : [{ path: scope, groups: [{ kind: 'table', objects: [ref] }] }],
            },
          })
        }),
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
          HttpResponse.json({ spec: tableSpec, editor: createTableEditor }),
        ),
        http.post(
          '/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations',
          async ({ request }) => {
            mutationBody = await request.json()
            return HttpResponse.json({
              applied: true,
              schema: { status: 'available', mode: 'ephemeral' },
              transaction: { mode: 'auto', open: false, pending_statements: 0 },
            })
          },
        ),
      )
      mockPermissions(['conn:ddl'])

      renderTree()
      expect(await screen.findByText('No objects.')).toBeInTheDocument()
      fireEvent.click(await screen.findByRole('button', { name: 'Create table' }))

      fireEvent.change(await screen.findByLabelText('Table name'), { target: { value: 'orders' } })
      fireEvent.change(screen.getByLabelText('Column name'), { target: { value: 'id' } })
      fireEvent.click(screen.getByRole('button', { name: /^create table$/i }))

      await waitFor(() =>
        expect(mutationBody).toEqual({
          operation: 'create_table',
          scope,
          name: 'orders',
          columns: [{ name: 'id', data_type: 'text', nullable: true, primary_key: false }],
        }),
      )
      await waitFor(() => expect(screen.queryByLabelText('Table name')).not.toBeInTheDocument())
      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      expect(await screen.findByText('orders')).toBeInTheDocument()
      expect(directoryRequests).toBeGreaterThanOrEqual(2)
    })

    it('does not offer create table in an empty scope without permission', async () => {
      store.getState().setSession(7, 'session-7')
      server.use(
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
          HttpResponse.json({
            directory: {
              connection: 'warehouse',
              dialect: 'postgres',
              database: 'analytics',
              generated_at: '',
              roots: [{ path: scope, groups: [] }],
            },
          }),
        ),
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
          HttpResponse.json({ spec: tableSpec, editor: createTableEditor }),
        ),
      )
      mockPermissions([])

      renderTree()
      expect(await screen.findByText('No objects.')).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Create table' })).not.toBeInTheDocument()
    })

    it('drops a table through the object context menu, showing cascade, and closes on success', async () => {
      store.getState().setSession(7, 'session-7')
      respondReady()
      let mutationBody: unknown
      server.use(
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
          HttpResponse.json({ spec: tableSpec, editor: dropEditor }),
        ),
        http.post(
          '/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations',
          async ({ request }) => {
            mutationBody = await request.json()
            return HttpResponse.json({
              applied: true,
              schema: { status: 'available', mode: 'ephemeral' },
              transaction: { mode: 'auto', open: false, pending_statements: 0 },
            })
          },
        ),
      )
      mockPermissions(['conn:ddl'])

      renderTree()
      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      const objectRow = await screen.findByRole('button', { name: 'orders' })
      fireEvent.contextMenu(objectRow)
      fireEvent.click(await screen.findByRole('menuitem', { name: /^Drop$/ }))

      expect(await screen.findByText('Cascade')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: /^drop$/i }))

      await waitFor(() =>
        expect(mutationBody).toEqual({ operation: 'drop_object', ref, cascade: false }),
      )
      await waitFor(() => expect(screen.queryByText('Cascade')).not.toBeInTheDocument())
    })

    it('never offers cascade when dropping an index, even when the driver supports cascade', async () => {
      store.getState().setSession(7, 'session-7')
      server.use(
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
          HttpResponse.json({
            directory: {
              connection: 'warehouse',
              dialect: 'postgres',
              database: 'analytics',
              generated_at: '',
              roots: [{ path: scope, groups: [{ kind: 'table', objects: [ref] }] }],
            },
          }),
        ),
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
          HttpResponse.json({ spec: tableSpec, editor: dropEditor }),
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
                  indexes: [{ name: 'orders_pkey', unique: true, columns: ['id'] }],
                },
              },
            ],
          }),
        ),
      )
      mockPermissions(['conn:ddl'])

      renderTree()
      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      fireEvent.click(await screen.findByRole('button', { name: 'orders' }))
      fireEvent.click(await screen.findByRole('button', { name: /Indexes/ }))
      const indexRow = await screen.findByText('orders_pkey')
      fireEvent.contextMenu(indexRow)
      fireEvent.click(await screen.findByRole('menuitem', { name: /Drop index/ }))

      expect(await screen.findByText(/Drop index/)).toBeInTheDocument()
      expect(screen.queryByText('Cascade')).not.toBeInTheDocument()
    })

    it('disables create/drop menu actions without a live session and never dispatches a request', async () => {
      respondReady()
      let mutationCalls = 0
      server.use(
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
          HttpResponse.json({
            spec: tableSpec,
            editor: {
              ...createTableEditor,
              ...dropEditor,
              operations: ['create_table', 'drop_object'],
            },
          }),
        ),
        http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations', () => {
          mutationCalls++
          return HttpResponse.json({
            applied: true,
            schema: { status: 'available', mode: 'ephemeral' },
            transaction: { mode: 'auto', open: false, pending_statements: 0 },
          })
        }),
      )
      mockPermissions(['conn:ddl'])

      renderTree()
      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      const objectRow = await screen.findByRole('button', { name: 'orders' })
      fireEvent.contextMenu(objectRow)
      fireEvent.click(await screen.findByRole('menuitem', { name: /Drop/ }))
      expect(screen.queryByText('Cascade')).not.toBeInTheDocument()
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      expect(mutationCalls).toBe(0)
    })
  })

  describe('Generate SQL', () => {
    const viewRef: ObjectRef = { scope, kind: 'view', name: 'orders_view' }

    function respondWithStatements(statements: {
      objects: { kind: string; operations: string[] }[]
    }) {
      server.use(
        http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/directory', () =>
          HttpResponse.json({
            directory: {
              connection: 'warehouse',
              dialect: 'postgres',
              database: 'analytics',
              generated_at: '',
              roots: [
                {
                  segment: scope[0],
                  path: scope,
                  groups: [
                    { kind: 'table', objects: [ref] },
                    { kind: 'view', objects: [viewRef] },
                  ],
                },
              ],
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
                {
                  kind: 'view',
                  label: 'View',
                  plural_label: 'Views',
                  order: 2,
                  relational: true,
                  supports_diagram: true,
                  listing: 'enumerated',
                },
              ],
            },
            statements,
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

    it('omits the Generate submenu when the spec advertises no operations', async () => {
      respondWithStatements({ objects: [] })
      renderTree()
      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      const objectRow = await screen.findByRole('button', { name: 'orders' })
      fireEvent.contextMenu(objectRow)
      expect(await screen.findByRole('menuitem', { name: 'Open' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Generate' })).not.toBeInTheDocument()
    })

    it('gives tables every advertised operation and views only their own', async () => {
      respondWithStatements({
        objects: [
          { kind: 'table', operations: ['select', 'insert', 'update', 'delete'] },
          { kind: 'view', operations: ['select'] },
        ],
      })
      renderTree()

      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      const tableRow = await screen.findByRole('button', { name: 'orders' })
      fireEvent.contextMenu(tableRow)
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Generate' }))
      expect(await screen.findByRole('menuitem', { name: 'Select' })).toBeInTheDocument()
      expect(screen.getByRole('menuitem', { name: 'Insert' })).toBeInTheDocument()
      expect(screen.getByRole('menuitem', { name: 'Update' })).toBeInTheDocument()
      expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()

      fireEvent.click(await screen.findByRole('button', { name: /Views/ }))
      const viewRow = await screen.findByRole('button', { name: 'orders_view' })
      fireEvent.contextMenu(viewRow)
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Generate' }))
      expect(await screen.findByRole('menuitem', { name: 'Select' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Insert' })).not.toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Update' })).not.toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
    })

    it('requests the generated statement and opens it in the Generate SQL dialog', async () => {
      respondWithStatements({ objects: [{ kind: 'table', operations: ['select'] }] })
      let requestBody: unknown
      server.use(
        http.post(
          '/api/v1/orgs/acme/workspaces/3/connections/7/schema/statements',
          async ({ request }) => {
            requestBody = await request.json()
            return HttpResponse.json({ sql: 'SELECT * FROM public.orders;' })
          },
        ),
      )

      renderTree()
      fireEvent.click(await screen.findByRole('button', { name: /Tables/ }))
      const objectRow = await screen.findByRole('button', { name: 'orders' })
      fireEvent.contextMenu(objectRow)
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Generate' }))
      fireEvent.click(await screen.findByRole('menuitem', { name: 'Select' }))

      expect(await screen.findByRole('heading', { name: 'Generate SQL' })).toBeInTheDocument()
      expect(await screen.findByTestId('generated-sql-preview')).toHaveTextContent(
        'SELECT * FROM public.orders;',
      )
      expect(requestBody).toEqual({ operation: 'select', ref })
    })
  })
})
