import { expect, it } from 'vitest'
import { decideCompletionPath, resolveLocalCompletions } from './resolve'
import type { CompletionIndex } from './schemaIndex'
import type { CursorContext } from './context'

const index: CompletionIndex = {
  version: 'v1',
  defaultSchema: 'public',
  schemas: ['public'],
  objects: [
    { schema: 'public', name: 'orders', kind: 'table' },
    { schema: 'public', name: 'order_items', kind: 'table' },
    { schema: 'public', name: 'active_orders', kind: 'view' },
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

it('resolves qualified refs locally when the alias is known', () => {
  const ctx: CursorContext = {
    positionClass: 'qualified',
    qualifier: 'o',
    fromRefs: [{ table: 'orders', schema: 'public', alias: 'o' }],
    cteNames: new Set(),
    prefix: 'to',
    protectedRegion: false,
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
  }
  expect(decideCompletionPath(ctx, index, false)).toBe('local-only')
  expect(resolveLocalCompletions(ctx, index).map((s) => s.label)).toEqual(
    expect.arrayContaining(['id', 'total']),
  )
})
