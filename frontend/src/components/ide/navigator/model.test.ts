import { describe, expect, it } from 'vitest'
import type { NavigatorListing, NavigatorNode, NavigatorTree, ScopePath } from '#/lib/api/types'
import { flattenNavigator, folderKey, objectKey, refOfPath, type ListingState } from './model'

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

const folder = (kind: string, label: string, child: string, order: number) => ({
  kind,
  label,
  child,
  order,
})

const tree: NavigatorTree = {
  system_objects: true,
  root: node('Connection', 'connection', {
    folders: [folder('databases', 'Databases', 'database', 10)],
  }),
  nodes: {
    database: node('Database', 'database', {
      scope: true,
      show_all_databases: true,
      folders: [folder('schemas', 'Schemas', 'schema', 10)],
    }),
    schema: node('Schema', 'schema', {
      scope: true,
      folders: [
        folder('tables', 'Tables', 'table', 10),
        folder('functions', 'Functions', 'function', 20),
      ],
    }),
    table: node('Table', 'table', {
      relational: true,
      folders: [
        folder('columns', 'Columns', 'column', 10),
        folder('references', 'References', 'reference', 20),
      ],
    }),
    column: node('Column', 'column', { leaf: true, column: true }),
    reference: node('Reference', 'reference', { leaf: true }),
    function: node('Function', 'function', { leaf: true }),
  },
}

const app: ScopePath = [{ kind: 'database', name: 'app' }]
const pub: ScopePath = [...app, { kind: 'schema', name: 'public' }]
const orders: ScopePath = [...pub, { kind: 'table', name: 'orders' }]

function listingOf(
  parent: ScopePath,
  folderKind: string,
  items: [string, string][],
): NavigatorListing {
  return {
    path: parent,
    folder: folderKind,
    items: items.map(([kind, name]) => ({
      kind,
      name,
      path: [...parent, { kind, name }],
      system: false,
      current: false,
    })),
    fetched_at: '2026-09-29T00:00:00Z',
    source: 'store',
  }
}

function cache(entries: [ScopePath, string, ListingState][]) {
  const map = new Map(entries.map(([p, f, s]) => [folderKey(p, f), s]))
  return (parent: ScopePath, folderKind: string) => map.get(folderKey(parent, folderKind))
}

const ready = (listing: NavigatorListing): ListingState => ({ status: 'ready', listing })

describe('flattenNavigator', () => {
  it('starts at the root folders and requests nothing while collapsed', () => {
    const result = flattenNavigator({ tree, isExpanded: () => false, listing: () => undefined })

    expect(result.rows).toEqual([
      expect.objectContaining({
        type: 'folder',
        depth: 0,
        expanded: false,
        key: folderKey([], 'databases'),
      }),
    ])
    expect(result.requests).toEqual([])
  })

  it('requests an expanded folder and shows a loading row until it resolves', () => {
    const expanded = new Set([folderKey([], 'databases')])
    const result = flattenNavigator({
      tree,
      isExpanded: (key) => expanded.has(key),
      listing: () => undefined,
    })

    expect(result.requests).toEqual([{ parent: [], folder: 'databases' }])
    expect(result.rows[1]).toEqual(
      expect.objectContaining({ type: 'status', status: 'loading', depth: 1 }),
    )
  })

  it('renders object rows from a listing and descends into expanded objects', () => {
    const databases = listingOf([], 'databases', [['database', 'app']])
    databases.items[0].current = true
    const expanded = new Set([
      folderKey([], 'databases'),
      objectKey(app),
      folderKey(app, 'schemas'),
    ])
    const result = flattenNavigator({
      tree,
      isExpanded: (key) => expanded.has(key),
      listing: cache([
        [[], 'databases', ready(databases)],
        [app, 'schemas', { status: 'session_required' }],
      ]),
    })

    expect(result.rows.map((row) => [row.type, row.depth])).toEqual([
      ['folder', 0],
      ['object', 1],
      ['folder', 2],
      ['status', 3],
    ])
    expect(result.rows[1]).toEqual(
      expect.objectContaining({
        type: 'object',
        expanded: true,
        item: expect.objectContaining({ current: true }),
      }),
    )
    expect(result.rows[3]).toEqual(expect.objectContaining({ status: 'session_required' }))
    expect(result.requests).toEqual([
      { parent: [], folder: 'databases' },
      { parent: app, folder: 'schemas' },
    ])
  })

  it('renders leaf kinds as leaf rows and an empty listing as an empty row', () => {
    const result = flattenNavigator({
      tree,
      isExpanded: () => true,
      listing: cache([
        [[], 'databases', ready(listingOf([], 'databases', [['database', 'app']]))],
        [app, 'schemas', ready(listingOf(app, 'schemas', [['schema', 'public']]))],
        [pub, 'tables', ready(listingOf(pub, 'tables', []))],
        [pub, 'functions', ready(listingOf(pub, 'functions', [['function', 'f1']]))],
      ]),
    })

    expect(result.rows.map((row) => row.type)).toEqual([
      'folder',
      'object',
      'folder',
      'object',
      'folder',
      'status',
      'folder',
      'leaf',
    ])
    expect(result.rows[5]).toEqual(expect.objectContaining({ status: 'empty', depth: 5 }))
  })

  it('keeps row keys unique when a listing repeats a kind and name', () => {
    const refs = listingOf(orders, 'references', [
      ['reference', 'fk_owner'],
      ['reference', 'fk_owner'],
    ])
    const result = flattenNavigator({
      tree: { ...tree, root: tree.nodes.table },
      isExpanded: () => true,
      listing: (_parent, folderKind) =>
        folderKind === 'references' ? ready(refs) : ready(listingOf(orders, folderKind, [])),
    })

    const keys = result.rows.map((row) => row.key)
    expect(new Set(keys).size).toBe(keys.length)
    expect(result.rows.filter((row) => row.type === 'leaf')).toHaveLength(2)
  })

  it('filters over cached listings only, forcing matched branches open without requests', () => {
    const listing = cache([
      [
        [],
        'databases',
        ready(
          listingOf([], 'databases', [
            ['database', 'app'],
            ['database', 'reports'],
          ]),
        ),
      ],
      [app, 'schemas', ready(listingOf(app, 'schemas', [['schema', 'public']]))],
      [
        pub,
        'tables',
        ready(
          listingOf(pub, 'tables', [
            ['table', 'orders'],
            ['table', 'users'],
          ]),
        ),
      ],
      [orders, 'columns', ready(listingOf(orders, 'columns', [['column', 'id']]))],
    ])
    const result = flattenNavigator({ tree, isExpanded: () => false, listing, filter: 'ORD' })

    expect(result.requests).toEqual([])
    const labels = result.rows.map((row) =>
      row.type === 'folder'
        ? `folder:${row.folder.kind}`
        : row.type === 'status'
          ? `status:${row.status}`
          : row.item.name,
    )
    expect(labels).toEqual([
      'folder:databases',
      'app',
      'folder:schemas',
      'public',
      'folder:tables',
      'orders',
    ])
  })

  it('stops filtering beneath an object whose own name matches', () => {
    const listing = cache([
      [[], 'databases', ready(listingOf([], 'databases', [['database', 'app']]))],
      [app, 'schemas', ready(listingOf(app, 'schemas', [['schema', 'public']]))],
      [pub, 'tables', ready(listingOf(pub, 'tables', [['table', 'orders']]))],
      [
        orders,
        'columns',
        ready(
          listingOf(orders, 'columns', [
            ['column', 'id'],
            ['column', 'total'],
          ]),
        ),
      ],
    ])
    const expanded = new Set([objectKey(orders), folderKey(orders, 'columns')])
    const result = flattenNavigator({
      tree,
      isExpanded: (key) => expanded.has(key),
      listing,
      filter: 'orders',
    })

    const leaves = result.rows.flatMap((row) => (row.type === 'leaf' ? [row.item.name] : []))
    expect(leaves).toEqual(['id', 'total'])
    expect(result.rows).toContainEqual(
      expect.objectContaining({
        type: 'folder',
        folder: expect.objectContaining({ kind: 'references' }),
      }),
    )
  })
})

describe('flattenNavigator column folders', () => {
  const listing = cache([
    [[], 'databases', ready(listingOf([], 'databases', [['database', 'app']]))],
    [app, 'schemas', ready(listingOf(app, 'schemas', [['schema', 'public']]))],
    [pub, 'tables', ready(listingOf(pub, 'tables', [['table', 'orders']]))],
    [orders, 'columns', ready(listingOf(orders, 'columns', [['column', 'id']]))],
  ])
  const opened = new Set([
    folderKey([], 'databases'),
    objectKey(app),
    folderKey(app, 'schemas'),
    objectKey(pub),
    folderKey(pub, 'tables'),
    objectKey(orders),
  ])

  it('opens the columns folder by default and requests its listing', () => {
    const result = flattenNavigator({
      tree,
      isExpanded: (key, defaultOpen) =>
        opened.has(key) || (!key.startsWith('object:') && defaultOpen),
      listing,
    })

    expect(result.rows).toContainEqual(expect.objectContaining({ type: 'leaf' }))
    expect(result.requests).toContainEqual({ parent: orders, folder: 'columns' })
  })

  it('keeps the columns folder collapsed once the user collapses it', () => {
    const result = flattenNavigator({
      tree,
      isExpanded: (key) => opened.has(key),
      listing,
    })

    expect(result.rows.some((row) => row.type === 'leaf')).toBe(false)
  })
})

describe('refOfPath', () => {
  it('splits a path into the owning scope and the object segment', () => {
    expect(refOfPath(orders)).toEqual({ scope: pub, kind: 'table', name: 'orders' })
  })
})
