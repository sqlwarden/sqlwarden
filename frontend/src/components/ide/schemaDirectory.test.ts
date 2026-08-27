import { describe, expect, it } from 'vitest'
import type { SchemaDirectory, SchemaSpec } from '#/lib/api/types'
import {
  filterDirectory,
  formatRowCount,
  hasDirectoryObjects,
  kindLabel,
  sortedGroups,
} from './schemaDirectory'

const spec: SchemaSpec = {
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
}

const directory: SchemaDirectory = {
  connection: 'c1',
  engine: 'postgres',
  default_scope: [{ kind: 'schema', name: 'public' }],
  generated_at: '',
  roots: [
    {
      path: [{ kind: 'schema', name: 'public' }],
      groups: [
        {
          kind: 'view',
          objects: [
            {
              scope: [{ kind: 'schema', name: 'public' }],
              kind: 'view',
              name: 'active_users',
            },
          ],
        },
        {
          kind: 'table',
          objects: [
            { scope: [{ kind: 'schema', name: 'public' }], kind: 'table', name: 'users' },
            { scope: [{ kind: 'schema', name: 'public' }], kind: 'table', name: 'orders' },
          ],
        },
      ],
    },
  ],
}

describe('kindLabel', () => {
  it('uses the schema spec plural label, falling back to a capitalized plural kind', () => {
    expect(kindLabel(spec, 'table')).toBe('Tables')
    expect(kindLabel(spec, 'sequence')).toBe('Sequences')
    expect(kindLabel(undefined, 'materialized_view')).toBe('Materialized Views')
  })

  it('replaces the temporary fallback with the backend plural label once schema spec loads', () => {
    const backendSpec: SchemaSpec = {
      dialect: 'test',
      kinds: [
        {
          kind: 'foo',
          label: 'Foo Resource',
          plural_label: 'Managed Foos',
          order: 1,
          relational: false,
          supports_diagram: false,
          listing: 'enumerated',
        },
      ],
    }

    expect(kindLabel(undefined, 'foo')).toBe('Foos')
    expect(kindLabel(backendSpec, 'foo')).toBe('Managed Foos')
  })
})

describe('sortedGroups', () => {
  it('orders groups by schema spec order', () => {
    const scope = directory.roots[0]
    const groups = sortedGroups(scope, spec)
    expect(groups.map((g) => g.kind)).toEqual(['table', 'view'])
  })
})

describe('filterDirectory', () => {
  it('returns the same reference for an empty query', () => {
    expect(filterDirectory(directory, '')).toBe(directory)
  })

  it('normalizes null collections from empty or older snapshots', () => {
    const empty = { ...directory, roots: null } as unknown as SchemaDirectory
    expect(filterDirectory(empty, '').roots).toEqual([])
    expect(filterDirectory(empty, 'users').roots).toEqual([])

    const nested = {
      ...directory,
      roots: [{ path: directory.roots[0].path, groups: null, children: null }],
    } as unknown as SchemaDirectory
    expect(filterDirectory(nested, '').roots).toEqual([
      { path: directory.roots[0].path, groups: [], children: undefined },
    ])
  })

  it('keeps only objects whose name matches, dropping empty groups/scopes', () => {
    const out = filterDirectory(directory, 'order')
    expect(out.roots).toHaveLength(1)
    const groups = out.roots[0].groups
    expect(groups).toHaveLength(1)
    expect(groups[0].kind).toBe('table')
    expect(groups[0].objects.map((o) => o.name)).toEqual(['orders'])
  })
})

describe('hasDirectoryObjects', () => {
  it('distinguishes empty scope nodes from directories containing objects', () => {
    expect(hasDirectoryObjects([{ path: [], groups: [] }])).toBe(false)
    expect(
      hasDirectoryObjects([
        {
          path: [],
          groups: [],
          children: [{ path: directory.roots[0].path, groups: directory.roots[0].groups }],
        },
      ]),
    ).toBe(true)
  })
})

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
