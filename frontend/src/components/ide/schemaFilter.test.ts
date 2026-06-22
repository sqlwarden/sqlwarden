import { describe, it, expect } from 'vitest'
import { filterSchema } from './schemaFilter'
import type { DbColumn, DbObject, DbObjectGroup, DbNamespace, DbSchema } from '#/lib/api/types'

const col = (name: string): DbColumn => ({ name, data_type: 'text', nullable: false, ordinal: 1 })
const obj = (name: string, columns: DbColumn[] = []): DbObject => ({ name, columns })
const grp = (kind: string, label: string, objects: DbObject[]): DbObjectGroup => ({ kind, label, objects })
const ns = (name: string, groups: DbObjectGroup[]): DbNamespace => ({ name, object_groups: groups })

const schema: DbSchema = {
  connection: '1',
  database: 'app',
  generated_at: '',
  namespaces: [
    ns('public', [
      grp('table', 'Tables', [obj('users', [col('id'), col('email')]), obj('orders', [col('id')])]),
      grp('view', 'Views', [obj('active_users', [col('id')])]),
    ]),
  ],
}

const tableNames = (out: DbSchema) =>
  (out.namespaces?.[0].object_groups ?? []).find((g) => g.kind === 'table')?.objects?.map((o) => o.name) ?? []
const viewNames = (out: DbSchema) =>
  (out.namespaces?.[0].object_groups ?? []).find((g) => g.kind === 'view')?.objects?.map((o) => o.name) ?? []

describe('filterSchema', () => {
  it('returns the schema unchanged for an empty query', () => {
    expect(filterSchema(schema, '   ')).toBe(schema)
  })

  it('keeps an object (with all columns) when its name matches', () => {
    const out = filterSchema(schema, 'order')
    expect(tableNames(out)).toEqual(['orders'])
    expect(viewNames(out)).toEqual([]) // view group dropped (no match)
  })

  it('keeps an object with only the matching column when a column matches', () => {
    const out = filterSchema(schema, 'email')
    expect(tableNames(out)).toEqual(['users'])
    const users = out.namespaces?.[0].object_groups?.[0].objects?.[0]
    expect(users?.columns?.map((c) => c.name)).toEqual(['email'])
  })

  it('matches objects in any group (e.g. views) by name', () => {
    const out = filterSchema(schema, 'active')
    expect(viewNames(out)).toEqual(['active_users'])
    expect(tableNames(out)).toEqual([])
  })

  it('drops empty groups and namespaces with no matches', () => {
    const out = filterSchema(schema, 'zzz-no-match')
    expect(out.namespaces).toEqual([])
  })
})
