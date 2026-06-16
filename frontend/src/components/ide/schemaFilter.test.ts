import { describe, it, expect } from 'vitest'
import { filterSchema } from './schemaFilter'
import type { DbSchema } from '#/lib/api/types'

const schema: DbSchema = {
  connection: '1',
  database: 'app',
  generated_at: '',
  namespaces: [
    {
      name: 'public',
      tables: [
        { name: 'users', columns: [{ name: 'id', data_type: 'bigint', nullable: false, ordinal: 1 }, { name: 'email', data_type: 'text', nullable: true, ordinal: 2 }] },
        { name: 'orders', columns: [{ name: 'id', data_type: 'bigint', nullable: false, ordinal: 1 }] },
      ],
      views: [{ name: 'active_users', columns: [{ name: 'id', data_type: 'bigint', nullable: false, ordinal: 1 }] }],
    },
  ],
}

describe('filterSchema', () => {
  it('returns the schema unchanged for an empty query', () => {
    expect(filterSchema(schema, '   ')).toBe(schema)
  })

  it('keeps a table (with all columns) when its name matches', () => {
    const out = filterSchema(schema, 'order')
    expect(out.namespaces?.[0].tables?.map((t) => t.name)).toEqual(['orders'])
    expect(out.namespaces?.[0].views ?? []).toEqual([])
  })

  it('keeps a table with only the matching column when a column matches', () => {
    const out = filterSchema(schema, 'email')
    const tables = out.namespaces?.[0].tables ?? []
    expect(tables.map((t) => t.name)).toEqual(['users'])
    expect(tables[0].columns?.map((c) => c.name)).toEqual(['email'])
  })

  it('matches views by name', () => {
    const out = filterSchema(schema, 'active')
    expect(out.namespaces?.[0].views?.map((v) => v.name)).toEqual(['active_users'])
    expect(out.namespaces?.[0].tables ?? []).toEqual([])
  })

  it('drops namespaces with no matches', () => {
    const out = filterSchema(schema, 'zzz-no-match')
    expect(out.namespaces).toEqual([])
  })
})
