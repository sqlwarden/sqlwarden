import { describe, expect, it } from 'vitest'
import type { SchemaCatalog, SchemaSpec } from '#/lib/api/types'
import { filterCatalog, kindLabel, sortedGroups } from './schemaCatalog'

const spec: SchemaSpec = {
  dialect: 'postgres',
  kinds: [
    { kind: 'table', label: 'Table', plural_label: 'Tables', order: 1, relational: true, supports_diagram: true, listing: 'enumerated' },
    { kind: 'view', label: 'View', plural_label: 'Views', order: 2, relational: true, supports_diagram: true, listing: 'enumerated' },
  ],
}

const catalog: SchemaCatalog = {
  connection: 'c1',
  dialect: 'postgres',
  database: 'app',
  generated_at: '',
  namespaces: [
    {
      name: 'public',
      groups: [
        { kind: 'view', objects: [{ namespace: 'public', kind: 'view', name: 'active_users' }] },
        {
          kind: 'table',
          objects: [
            { namespace: 'public', kind: 'table', name: 'users' },
            { namespace: 'public', kind: 'table', name: 'orders' },
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
    const ns = catalog.namespaces![0]
    const groups = sortedGroups(ns, spec)
    expect(groups.map((g) => g.kind)).toEqual(['table', 'view'])
  })
})

describe('filterCatalog', () => {
  it('returns the same reference for an empty query', () => {
    expect(filterCatalog(catalog, '')).toBe(catalog)
  })

  it('keeps only objects whose name matches, dropping empty groups/namespaces', () => {
    const out = filterCatalog(catalog, 'order')
    expect(out.namespaces).toHaveLength(1)
    const groups = out.namespaces![0].groups!
    expect(groups).toHaveLength(1)
    expect(groups[0].kind).toBe('table')
    expect(groups[0].objects.map((o) => o.name)).toEqual(['orders'])
  })
})
