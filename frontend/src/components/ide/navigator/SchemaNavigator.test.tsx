import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ContextMenuProvider } from '#/components/ui/context-menu'
import { IconPackProvider } from '#/lib/icons'
import { queryKeys } from '#/lib/api/query-keys'
import type {
  NavigatorListing,
  NavigatorNode,
  SchemaTreeResponse,
  ScopePath,
} from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createEditorViewRegistry, EditorViewRegistryContext } from '../useEditorViewRegistry'
import { createIdeStore, IdeStoreContext } from '../useIdeStore'
import { formatRowCount } from './NavigatorRow'
import { folderKey, objectKey } from './model'
import { SchemaNavigator } from './SchemaNavigator'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({
    count,
    estimateSize,
  }: {
    count: number
    estimateSize: (i: number) => number
  }) => {
    let offset = 0
    const items = Array.from({ length: count }, (_, index) => {
      const size = estimateSize(index)
      const item = { index, start: offset, end: offset + size, size, key: index, lane: 0 }
      offset += size
      return item
    })
    return { getTotalSize: () => offset, getVirtualItems: () => items, measureElement: vi.fn() }
  },
}))

const base = '/api/v1/orgs/acme/workspaces/3/connections/7/schema'

function node(label: string, icon: string, overrides: Partial<NavigatorNode> = {}): NavigatorNode {
  return {
    label,
    icon,
    leaf: false,
    folders: [],
    scope: false,
    relational: false,
    supports_diagram: false,
    has_definition: false,
    show_all_databases: false,
    column: false,
    ...overrides,
  }
}

const tree: SchemaTreeResponse = {
  system_objects: true,
  root: node('Connection', 'connection', {
    folders: [{ kind: 'databases', label: 'Databases', child: 'database', order: 10 }],
  }),
  nodes: {
    database: node('Database', 'database', {
      scope: true,
      folders: [{ kind: 'schemas', label: 'Schemas', child: 'schema', order: 10 }],
    }),
    schema: node('Schema', 'schema', {
      scope: true,
      folders: [{ kind: 'tables', label: 'Tables', child: 'table', order: 10 }],
    }),
    table: node('Table', 'table', {
      relational: true,
      folders: [{ kind: 'columns', label: 'Columns', child: 'column', order: 10 }],
    }),
    column: node('Column', 'column', { leaf: true, column: true }),
  },
}

const app: ScopePath = [{ kind: 'database', name: 'app' }]
const pub: ScopePath = [...app, { kind: 'schema', name: 'public' }]
const orders: ScopePath = [...pub, { kind: 'table', name: 'orders' }]

function listing(
  path: ScopePath,
  folder: string,
  items: {
    kind: string
    name: string
    attributes?: Record<string, unknown>
    system?: boolean
    current?: boolean
  }[],
): NavigatorListing {
  return {
    path,
    folder,
    items: items.map((entry) => ({
      kind: entry.kind,
      name: entry.name,
      path: [...path, { kind: entry.kind, name: entry.name }],
      attributes: entry.attributes,
      system: entry.system ?? false,
      current: entry.current ?? false,
    })),
    fetched_at: new Date().toISOString(),
    source: 'store',
  }
}

describe('formatRowCount', () => {
  it('shows exact counts under 1000', () => {
    expect(formatRowCount(0)).toBe('0')
    expect(formatRowCount(1)).toBe('1')
    expect(formatRowCount(999)).toBe('999')
  })

  it('compacts thousands and millions with one decimal', () => {
    expect(formatRowCount(1200)).toBe('~1.2K')
    expect(formatRowCount(2_500_000)).toBe('~2.5M')
  })
})

describe('SchemaNavigator', () => {
  let store: ReturnType<typeof createIdeStore>
  let nodeRequests: URL[]

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    nodeRequests = []
    server.use(
      http.get(`${base}/tree`, () => HttpResponse.json(tree)),
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({ resource_type: 'connection', resource_id: 7, permissions: [] }),
      ),
      http.get(`${base}/nodes`, ({ request }) => {
        const url = new URL(request.url)
        nodeRequests.push(url)
        if (url.searchParams.get('folder') === 'databases') {
          return HttpResponse.json(
            listing([], 'databases', [
              { kind: 'database', name: 'app', current: true },
              { kind: 'database', name: 'template1', system: true },
            ]),
          )
        }
        return HttpResponse.json(
          {
            error: {
              code: 'session_required',
              message: 'Connect to this database to load schema objects.',
            },
          },
          { status: 409 },
        )
      }),
    )
  })

  function renderNavigator(filter = '', queryClient = createTestQueryClient()) {
    const onConnect = vi.fn()
    render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <EditorViewRegistryContext.Provider value={createEditorViewRegistry()}>
            <IconPackProvider>
              <ContextMenuProvider>
                <SchemaNavigator
                  orgSlug="acme"
                  workspaceId={3}
                  connectionId={7}
                  driver="postgres"
                  filter={filter}
                  onConnect={onConnect}
                />
              </ContextMenuProvider>
            </IconPackProvider>
          </EditorViewRegistryContext.Provider>
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
    return { onConnect, queryClient }
  }

  const expand = (key: string) => store.getState().setNodeExpanded(`nav:7:${key}`, true)

  it('renders the grammar root from /tree without requesting any listing', async () => {
    renderNavigator()

    expect(await screen.findByRole('treeitem', { name: /Databases/ })).toBeInTheDocument()
    expect(nodeRequests).toHaveLength(0)
  })

  it('loads a folder on expand, bolding the current database and dimming system ones', async () => {
    renderNavigator()
    fireEvent.click(await screen.findByRole('treeitem', { name: /Databases/ }))

    const current = await screen.findByText('app')
    expect(current).toHaveClass('font-semibold')
    expect(screen.getByRole('treeitem', { name: /template1/ })).toHaveClass('text-muted-foreground')
    expect(nodeRequests).toHaveLength(1)
    expect(nodeRequests[0].searchParams.has('path')).toBe(false)
  })

  it('renders a connect row when a listing needs a session', async () => {
    expand(folderKey([], 'databases'))
    expand(objectKey(app))
    expand(folderKey(app, 'schemas'))
    const { onConnect } = renderNavigator()

    fireEvent.click(await screen.findByRole('button', { name: 'Connect to load' }))
    expect(onConnect).toHaveBeenCalled()
    expect(nodeRequests.at(-1)?.searchParams.get('path')).toBe(JSON.stringify(app))
  })

  it('shows column types with key badges', async () => {
    const queryClient = createTestQueryClient()
    const cache = (path: ScopePath, folder: string, value: NavigatorListing) =>
      queryClient.setQueryData(queryKeys.connectionSchemaNodes('acme', 3, 7, path, folder), value)
    cache([], 'databases', listing([], 'databases', [{ kind: 'database', name: 'app' }]))
    cache(app, 'schemas', listing(app, 'schemas', [{ kind: 'schema', name: 'public' }]))
    cache(pub, 'tables', listing(pub, 'tables', [{ kind: 'table', name: 'orders' }]))
    cache(
      orders,
      'columns',
      listing(orders, 'columns', [
        {
          kind: 'column',
          name: 'id',
          attributes: { data_type: 'bigint', nullable: false, primary_key: true },
        },
      ]),
    )
    for (const key of [
      folderKey([], 'databases'),
      objectKey(app),
      folderKey(app, 'schemas'),
      objectKey(pub),
      folderKey(pub, 'tables'),
      objectKey(orders),
      folderKey(orders, 'columns'),
    ]) {
      expand(key)
    }
    renderNavigator('', queryClient)

    const column = await screen.findByRole('treeitem', { name: /id/ })
    expect(column).toHaveTextContent('PK')
    expect(column).toHaveTextContent('bigint')
  })

  it('refreshes an object path with a session and connects without one', async () => {
    let refreshBody: unknown
    server.use(
      http.post(`${base}/refresh`, async ({ request }) => {
        refreshBody = await request.json()
        return HttpResponse.json({ items: [] })
      }),
    )
    expand(folderKey([], 'databases'))
    const { onConnect } = renderNavigator()

    fireEvent.click(await screen.findByRole('button', { name: 'Refresh app' }))
    expect(onConnect).toHaveBeenCalled()
    expect(refreshBody).toBeUndefined()

    store.getState().setSession(7, 'session-7')
    fireEvent.click(await screen.findByRole('button', { name: 'Refresh app' }))
    await waitFor(() => expect(refreshBody).toEqual({ path: app }))
  })

  it('leaves the row collapsed when Enter is pressed on its refresh button', async () => {
    expand(folderKey([], 'databases'))
    renderNavigator()

    const row = await screen.findByRole('treeitem', { name: /^app/ })
    fireEvent.keyDown(screen.getByRole('button', { name: 'Refresh app' }), { key: 'Enter' })
    expect(row).toHaveAttribute('aria-expanded', 'false')
  })

  it('filters loaded objects only, revealing cached matches without new requests', async () => {
    const queryClient = createTestQueryClient()
    queryClient.setQueryData(
      queryKeys.connectionSchemaNodes('acme', 3, 7, [], 'databases'),
      listing([], 'databases', [{ kind: 'database', name: 'app' }]),
    )
    queryClient.setQueryData(
      queryKeys.connectionSchemaNodes('acme', 3, 7, app, 'schemas'),
      listing(app, 'schemas', [{ kind: 'schema', name: 'reporting' }]),
    )
    renderNavigator('report', queryClient)

    expect(await screen.findByText('reporting')).toBeInTheDocument()
    expect(screen.getByText('Filter matches loaded objects only.')).toBeInTheDocument()
    expect(nodeRequests).toHaveLength(0)
  })
})
