import { describe, expect, it } from 'vitest'
import { decideCompletionPath, resolveLocalCompletions } from './resolve'
import { rankSuggestions } from './rank'
import type { CompletionIndex } from './schemaIndex'
import { postgresDialect } from '../engines/postgres/dialect'
import { sqlServerDialect } from '../engines/sqlserver/dialect'
import { classifyCursorContext, type CursorContext } from './context'

const index: CompletionIndex = {
  version: 'v1',
  defaultSchema: 'public',
  searchSchemas: ['public'],
  defaultScopeRelationsListed: true,
  columnScore: 100,
  schemas: ['public'],
  objects: [
    { schema: 'public', name: 'orders', kind: 'table', score: 90 },
    { schema: 'public', name: 'order_items', kind: 'table', score: 90 },
    { schema: 'public', name: 'active_orders', kind: 'view', score: 85 },
    { schema: 'public', name: 'aaa_orders_seq', kind: 'sequence', score: 65 },
  ],
  columnsByTable: new Map([
    [
      'public orders',
      [
        { schema: 'public', table: 'orders', name: 'id', type: 'int8', nullable: false },
        { schema: 'public', table: 'orders', name: 'total', type: 'numeric', nullable: true },
      ],
    ],
    [
      'orders',
      [
        { schema: 'public', table: 'orders', name: 'id', type: 'int8', nullable: false },
        { schema: 'public', table: 'orders', name: 'total', type: 'numeric', nullable: true },
      ],
    ],
  ]),
  allColumns: [
    { schema: 'public', table: 'orders', name: 'id', type: 'int8', nullable: false },
    { schema: 'public', table: 'orders', name: 'total', type: 'numeric', nullable: true },
  ],
}

const base: CursorContext = {
  positionClass: 'relation',
  fromRefs: [],
  cteNames: new Set(),
  prefix: 'ord',
  protectedRegion: false,
  openQuotedIdentifier: false,
  afterTableRef: false,
  afterClauseLead: false,
}

it('serves relation positions locally', () => {
  expect(decideCompletionPath(base, index, false)).toBe('local-only')
  const out = resolveLocalCompletions(base, index)
  expect(out.map((s) => s.label)).toEqual(
    expect.arrayContaining(['orders', 'order_items', 'active_orders']),
  )
  expect(out.find((s) => s.label === 'orders')?.namespace).toBe('public')
})

it('serves keyword positions locally', () => {
  const ctx = { ...base, positionClass: 'keyword' as const, prefix: 'sel' }
  expect(decideCompletionPath(ctx, index, false)).toBe('local-only')
})

it('offers no schema objects or columns at keyword positions after a table reference', () => {
  const ctx: CursorContext = {
    positionClass: 'keyword',
    fromRefs: [{ table: 'orders' }],
    cteNames: new Set(),
    prefix: '',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: true,
    afterClauseLead: false,
  }
  expect(resolveLocalCompletions(ctx, index)).toEqual([])
  expect(resolveLocalCompletions({ ...ctx, prefix: 'ord' }, index)).toEqual([])
  const sql = 'select * from orders '
  expect(resolveLocalCompletions(classifyCursorContext(sql, sql.length), index)).toEqual([])
})

it('resolves qualified refs locally when the alias is known', () => {
  const ctx: CursorContext = {
    positionClass: 'qualified',
    qualifier: 'o',
    fromRefs: [{ table: 'orders', schema: 'public', alias: 'o' }],
    cteNames: new Set(),
    prefix: 'to',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: false,
    afterClauseLead: false,
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('local-only')
  expect(resolveLocalCompletions(ctx, index).map((s) => s.label)).toEqual(['total'])
})

it('falls back to the backend for qualified refs with an unknown alias', () => {
  const ctx: CursorContext = {
    positionClass: 'qualified',
    qualifier: 'x',
    fromRefs: [],
    cteNames: new Set(),
    prefix: '',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: false,
    afterClauseLead: false,
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
})

it('falls back to the backend for column positions with no resolvable FROM refs', () => {
  const ctx = { ...base, positionClass: 'column' as const, fromRefs: [], prefix: 'id' }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
})

it('uses column FROM refs locally when they resolve', () => {
  const ctx: CursorContext = {
    positionClass: 'column',
    fromRefs: [{ table: 'orders', schema: 'public', alias: 'o' }],
    cteNames: new Set(),
    prefix: 'to',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: false,
    afterClauseLead: false,
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('local-only')
  expect(resolveLocalCompletions(ctx, index).map((s) => s.label)).toContain('total')
})

it('falls back to the backend for a relation position when a CTE is defined', () => {
  const ctx = { ...base, cteNames: new Set(['recent_orders']) }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
  expect(decideCompletionPath(ctx, index, true)).toBe('backend')
})

it('falls back to the backend for column refs that resolve to a CTE name', () => {
  const ctx: CursorContext = {
    positionClass: 'column',
    fromRefs: [{ table: 'recent_orders' }],
    cteNames: new Set(['recent_orders']),
    prefix: '',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: false,
    afterClauseLead: false,
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
  expect(resolveLocalCompletions(ctx, index)).toEqual([])
})

it('always offers object candidates for a typed prefix even from an unknown position', () => {
  const ctx = { ...base, positionClass: 'unknown' as const, prefix: 'ord' }
  const out = resolveLocalCompletions(ctx, index)
  expect(out.map((s) => s.label)).toEqual(expect.arrayContaining(['orders', 'order_items']))
})

it('routes explicit invokes through local-then-backend', () => {
  expect(decideCompletionPath(base, index, true)).toBe('local-then-backend')
})

it('routes everything to the backend when the index is null', () => {
  expect(decideCompletionPath(base, null, false)).toBe('backend')
})

it('returns nothing for protected regions', () => {
  const ctx = { ...base, protectedRegion: true }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend') // source.ts bails before calling; guard anyway
  expect(resolveLocalCompletions(ctx, index)).toEqual([])
})

it('routes value positions to the backend and offers nothing locally', () => {
  const ctx = { ...base, positionClass: 'value' as const, prefix: '' }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
  expect(decideCompletionPath(ctx, index, true)).toBe('backend')
  expect(resolveLocalCompletions(ctx, index)).toEqual([])
})

it('resolves an INSERT column list against the target table locally', () => {
  const ctx: CursorContext = {
    positionClass: 'column',
    fromRefs: [{ table: 'orders' }],
    cteNames: new Set(),
    prefix: '',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: false,
    afterClauseLead: false,
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('local-only')
  expect(resolveLocalCompletions(ctx, index).map((s) => s.label)).toEqual(
    expect.arrayContaining(['id', 'total']),
  )
})

const emptyIndex: CompletionIndex = {
  version: 'v0',
  defaultSchema: 'public',
  searchSchemas: ['public'],
  defaultScopeRelationsListed: false,
  columnScore: 100,
  schemas: [],
  objects: [],
  columnsByTable: new Map(),
  allColumns: [],
}

it('routes a relation position to the backend when the index has no objects', () => {
  const ctx = { ...base, prefix: '' }
  expect(decideCompletionPath(ctx, emptyIndex, false)).toBe('backend')
  expect(decideCompletionPath(ctx, emptyIndex, true)).toBe('backend')
})

it('routes a relation position to the backend when no indexed object matches the typed prefix', () => {
  const ctx = { ...base, prefix: 'zzz' }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
  expect(decideCompletionPath(ctx, index, true)).toBe('backend')
})

it('keeps an empty-prefix relation position local when the default scope relations are listed', () => {
  expect(decideCompletionPath({ ...base, prefix: '' }, index, false)).toBe('local-only')
})

it('routes a relation position to the backend when the default scope relations are not listed', () => {
  const unlisted = { ...index, defaultScopeRelationsListed: false }
  expect(decideCompletionPath({ ...base, prefix: '' }, unlisted, false)).toBe('backend')
  expect(decideCompletionPath(base, unlisted, true)).toBe('backend')
})

it('routes a relation position to the backend when only non-default-schema objects exist', () => {
  const otherSchemas: CompletionIndex = {
    ...index,
    objects: [{ schema: 'analytics', name: 'ordinals', kind: 'table', score: 90 }],
  }
  expect(decideCompletionPath({ ...base, prefix: '' }, otherSchemas, false)).toBe('local-only')
  expect(decideCompletionPath(base, otherSchemas, false)).toBe('backend')
  expect(decideCompletionPath(base, otherSchemas, true)).toBe('backend')
})

it('projects the index column score onto local column suggestions', () => {
  const ctx: CursorContext = { ...base, positionClass: 'column', fromRefs: [{ table: 'orders' }] }
  const out = resolveLocalCompletions(ctx, { ...index, columnScore: 77 })
  expect(out.every((s) => s.score === 77)).toBe(true)
})

it('offers objects and columns at unknown positions with columns ranked first', () => {
  const ctx = { ...base, positionClass: 'unknown' as const, prefix: 'o' }
  const out = resolveLocalCompletions(ctx, index)
  const column = out.find((s) => s.kind === 'column')
  const table = out.find((s) => s.kind === 'table')
  expect(column!.score).toBeGreaterThan(table!.score!)
})

it.each([
  ['UPDATE orders ', true],
  ['INSERT INTO orders ', true],
  ['SELECT * FROM orders o ', true],
  ['SELECT * FROM ONLY ', false],
  ['SELECT * FROM a, LATERAL ', false],
])('classifies %j afterTableRef as %s', (sql, expected) => {
  const ctx = classifyCursorContext(sql, sql.length)
  expect(ctx.afterTableRef).toBe(expected)
  if (expected) expect(resolveLocalCompletions(ctx, index)).toEqual([])
  else expect(ctx.positionClass).toBe('relation')
})

it('records no phantom relations for ONLY and LATERAL', () => {
  const only = 'SELECT * FROM ONLY orders WHERE '
  expect(classifyCursorContext(only, only.length).fromRefs).toEqual([{ table: 'orders' }])
  const lateral = 'SELECT * FROM a, LATERAL '
  expect(classifyCursorContext(lateral, lateral.length).fromRefs).toEqual([{ table: 'a' }])
})

it('offers objects for an identifier typed above an existing FROM', () => {
  const sql = 'ord\nFROM orders'
  const cursor = sql.indexOf('ord') + 3
  const ctx = classifyCursorContext(sql, cursor)
  expect(ctx.afterTableRef).toBe(false)
  expect(resolveLocalCompletions(ctx, index).map((s) => s.label)).toContain('orders')
})

it('still offers no objects after ORDER or GROUP', () => {
  const sql = 'SELECT * FROM orders ORDER '
  const ctx = classifyCursorContext(sql, sql.length)
  expect(ctx.afterClauseLead).toBe(true)
  expect(resolveLocalCompletions(ctx, index)).toEqual([])
})

it('routes a qualified ref to the backend when its table has no indexed columns', () => {
  const ctx: CursorContext = {
    positionClass: 'qualified',
    qualifier: 'o',
    fromRefs: [{ table: 'unexpanded', schema: 'public', alias: 'o' }],
    cteNames: new Set(),
    prefix: '',
    protectedRegion: false,
    openQuotedIdentifier: false,
    afterTableRef: false,
    afterClauseLead: false,
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('backend')
  expect(decideCompletionPath(ctx, index, true)).toBe('backend')
})

it('orders local relation candidates by object kind before name', () => {
  const sql = 'select * from '
  const ranked = rankSuggestions(
    resolveLocalCompletions(classifyCursorContext(sql, sql.length), index),
    '',
    'relation',
  )
  expect(ranked.map((s) => s.label)).toEqual([
    'order_items',
    'orders',
    'active_orders',
    'aaa_orders_seq',
  ])
})

const searchIndex: CompletionIndex = {
  ...index,
  defaultSchema: 'sales',
  searchSchemas: ['sales', 'dbo'],
  schemas: ['sales', 'dbo', 'audit'],
  objects: [
    { schema: 'sales', name: 'orders', kind: 'table', score: 90 },
    { schema: 'dbo', name: 'Orders', kind: 'table', score: 90 },
    { schema: 'dbo', name: 'customers', kind: 'table', score: 90 },
    { schema: 'audit', name: 'orders', kind: 'table', score: 90 },
  ],
}

const relationCtx: CursorContext = { ...base, positionClass: 'relation', prefix: 'cust' }

it('covers relations found in a fallback search schema', () => {
  expect(decideCompletionPath(relationCtx, searchIndex, false)).toBe('local-only')
  expect(decideCompletionPath({ ...relationCtx, prefix: 'zzz' }, searchIndex, false)).toBe(
    'backend',
  )
})

it('hides relations shadowed by an earlier search schema', () => {
  const owners = resolveLocalCompletions({ ...relationCtx, prefix: '' }, searchIndex)
    .map((s) => `${s.namespace}.${s.label}`)
    .sort()
  expect(owners).toEqual(['audit.orders', 'dbo.customers', 'sales.orders'])
})

it('matches empty-schema objects for flat engines', () => {
  const flat: CompletionIndex = {
    ...index,
    defaultSchema: '',
    searchSchemas: [''],
    objects: [{ schema: '', name: 'customers', kind: 'table', score: 90 }],
  }
  expect(decideCompletionPath(relationCtx, flat, false)).toBe('local-only')
  expect(resolveLocalCompletions({ ...relationCtx, prefix: '' }, flat).map((s) => s.label)).toEqual(
    ['customers'],
  )
})

describe('dialect quoting of local suggestions', () => {
  const quotedIndex: CompletionIndex = {
    ...index,
    defaultSchema: 'dbo',
    searchSchemas: ['dbo'],
    schemas: ['dbo'],
    objects: [
      { schema: 'dbo', name: 'order details', kind: 'table', score: 90 },
      { schema: 'dbo', name: 'orders', kind: 'table', score: 90 },
      { schema: 'dbo', name: 'Invoices', kind: 'table', score: 90 },
    ],
    columnsByTable: new Map([
      [
        'orders',
        [
          { schema: 'dbo', table: 'orders', name: 'unit price', type: 'int', nullable: false },
          { schema: 'dbo', table: 'orders', name: 'id', type: 'int', nullable: false },
        ],
      ],
    ]),
    allColumns: [],
  }

  it('brackets relation names that need quoting under SQL Server', () => {
    const out = resolveLocalCompletions(base, quotedIndex, sqlServerDialect)
    const byLabel = new Map(out.map((s) => [s.label, s.insert_text]))
    expect(byLabel.get('order details')).toBe('[order details]')
    expect(byLabel.get('orders')).toBeUndefined()
  })

  it('quotes column names that need quoting', () => {
    const ctx: CursorContext = {
      ...base,
      positionClass: 'column',
      prefix: '',
      fromRefs: [{ table: 'orders' }],
    }
    const out = resolveLocalCompletions(ctx, quotedIndex, sqlServerDialect)
    const byLabel = new Map(out.map((s) => [s.label, s.insert_text]))
    expect(byLabel.get('unit price')).toBe('[unit price]')
    expect(byLabel.get('id')).toBeUndefined()
  })

  it('uses the dialect quote style and case rules', () => {
    const out = resolveLocalCompletions(base, quotedIndex, postgresDialect)
    const byLabel = new Map(out.map((s) => [s.label, s.insert_text]))
    expect(byLabel.get('order details')).toBe('"order details"')
    expect(byLabel.get('Invoices')).toBe('"Invoices"')
    expect(byLabel.get('orders')).toBeUndefined()
  })
})
