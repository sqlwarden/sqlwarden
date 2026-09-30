import { describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '#/components/ui/context-menu'
import type { NavigatorItem, NavigatorNode, SchemaTreeResponse, ScopePath } from '#/lib/api/types'
import { dialectFor } from '../sqlDialect'
import type { NavigatorRow } from './model'
import { navigatorMenu, refreshPathOf, type NavigatorActions } from './navigatorMenus'

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
  system_objects: false,
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
      folders: [
        { kind: 'tables', label: 'Tables', child: 'table', order: 10 },
        { kind: 'functions', label: 'Functions', child: 'function', order: 20 },
      ],
    }),
    table: node('Table', 'table', {
      relational: true,
      supports_diagram: true,
      folders: [{ kind: 'columns', label: 'Columns', child: 'column', order: 10 }],
    }),
    column: node('Column', 'column', { leaf: true, column: true }),
    function: node('Function', 'function', { leaf: true, has_definition: true }),
  },
}

const app: ScopePath = [{ kind: 'database', name: 'app' }]
const pub: ScopePath = [...app, { kind: 'schema', name: 'public' }]
const orders: ScopePath = [...pub, { kind: 'table', name: 'orders' }]

function item(path: ScopePath, attributes?: Record<string, unknown>): NavigatorItem {
  const last = path[path.length - 1]
  return { kind: last.kind, name: last.name, path, attributes, system: false, current: false }
}

function actions(overrides: Partial<NavigatorActions> = {}): NavigatorActions {
  return {
    tree,
    dialect: dialectFor('postgres'),
    sessionId: undefined,
    canMutate: false,
    refresh: vi.fn(),
    openObject: vi.fn(),
    openDiagram: vi.fn(),
    columnsOf: () => [],
    edit: {
      createTable: vi.fn(),
      dropScope: vi.fn(),
      dropObject: vi.fn(),
      editColumn: vi.fn(),
      renameColumn: vi.fn(),
      dropColumn: vi.fn(),
      createIndex: vi.fn(),
      dropIndex: vi.fn(),
      generateStatement: vi.fn(),
    },
    ...overrides,
  }
}

const ids = (items: ContextMenuItem[]) =>
  items.flatMap((entry) => ('id' in entry && entry.id ? [entry.id] : []))

const objectRow = (path: ScopePath, kind: string): NavigatorRow => ({
  type: 'object',
  key: path.map((s) => s.name).join('/'),
  depth: path.length,
  item: item(path),
  node: tree.nodes[kind],
  folder: { kind: `${kind}s`, label: '', child: kind, order: 0 },
  expanded: false,
})

const leafRow = (
  path: ScopePath,
  kind: string,
  attributes?: Record<string, unknown>,
): NavigatorRow => ({
  type: 'leaf',
  key: path.map((s) => s.name).join('/'),
  depth: path.length,
  item: item(path, attributes),
  node: tree.nodes[kind],
  folder: { kind: `${kind}s`, label: '', child: kind, order: 0 },
})

describe('refreshPathOf', () => {
  it('refreshes objects at their own path and leaves and folders at the parent', () => {
    expect(refreshPathOf(objectRow(orders, 'table'))).toEqual(orders)
    expect(refreshPathOf(leafRow([...orders, { kind: 'column', name: 'id' }], 'column'))).toEqual(
      orders,
    )
    expect(
      refreshPathOf({
        type: 'folder',
        key: 'f',
        depth: 0,
        parent: pub,
        folder: tree.nodes.schema.folders[0],
        expanded: false,
      }),
    ).toEqual(pub)
  })
})

describe('navigatorMenu', () => {
  it('offers a schema diagram only on scopes whose folders hold diagram kinds', () => {
    expect(ids(navigatorMenu(objectRow(pub, 'schema'), actions()))).toContain('view-schema-diagram')
    expect(ids(navigatorMenu(objectRow(app, 'database'), actions()))).not.toContain(
      'view-schema-diagram',
    )
  })

  it('labels the scope drop action with the grammar label', () => {
    const drop = navigatorMenu(objectRow(app, 'database'), actions()).find(
      (entry) => 'id' in entry && entry.id === 'drop-schema',
    )
    expect(drop && 'label' in drop ? drop.label : undefined).toBe('Drop database')
  })

  it('routes the object Refresh item to the object path', () => {
    const refresh = vi.fn()
    const menu = navigatorMenu(objectRow(orders, 'table'), actions({ refresh }))
    const entry = menu.find((e) => 'id' in e && e.id === 'refresh')
    if (entry && 'onSelect' in entry) entry.onSelect?.()
    expect(refresh).toHaveBeenCalledWith(orders)
  })

  it('gives object-level leaves the object menu and column leaves the column menu plus refresh', () => {
    const fn = leafRow([...pub, { kind: 'function', name: 'f1' }], 'function')
    expect(ids(navigatorMenu(fn, actions()))).toContain('open')

    const column = leafRow([...orders, { kind: 'column', name: 'id' }], 'column', {
      data_type: 'bigint',
    })
    const columnIds = ids(navigatorMenu(column, actions()))
    expect(columnIds).toEqual(expect.arrayContaining(['copy-column-name', 'copy-type', 'refresh']))
  })
})
