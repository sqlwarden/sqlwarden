import { describe, it, expect } from 'vitest'
import {
  refKey,
  hiddenNeighbors,
  rankByDegree,
  reachableRefs,
  planNamespaceSeed,
  estimateNodeSize,
  edgeCardinality,
  relationshipHandleId,
  DIAGRAM_MAX_TABLES,
} from './diagramModel'
import type { ObjectRef, Relationship, ObjectDetail } from '#/lib/api/types'

const t = (name: string): ObjectRef => ({ namespace: 'public', kind: 'table', name })
const edge = (from: string, to: string): Relationship => ({
  name: `${from}_${to}_fk`,
  source: t(from),
  columns: ['x'],
  references: t(to),
  referenced_columns: ['id'],
})
const edges: Relationship[] = [
  edge('orders', 'users'),
  edge('orders', 'products'),
  edge('reviews', 'products'),
]

describe('diagramModel', () => {
  it('refKey is stable and namespace+kind+name scoped', () => {
    expect(refKey(t('users'))).toBe('public table users')
  })

  it('hiddenNeighbors returns both directions excluding those already present', () => {
    const present = new Set([refKey(t('orders'))])
    const got = hiddenNeighbors(t('orders'), edges, present)
      .map((r) => r.name)
      .sort()
    expect(got).toEqual(['products', 'users'])
  })

  it('rankByDegree orders by FK-connection count desc', () => {
    const ranked = rankByDegree([t('users'), t('orders'), t('products'), t('reviews')], edges).map(
      (r) => r.name,
    )
    expect(ranked.slice(0, 2).sort()).toEqual(['orders', 'products'])
  })

  it('reachableRefs follows FK edges transitively across the whole component', () => {
    // orders→users, orders→products, reviews→products: from orders you can reach
    // reviews transitively through products.
    const got = reachableRefs([t('orders')], edges)
      .map((r) => r.name)
      .sort()
    expect(got).toEqual(['orders', 'products', 'reviews', 'users'])
  })

  it('reachableRefs is bounded by maxTables (BFS keeps the seed nearest first)', () => {
    const got = reachableRefs([t('orders')], edges, 2)
    expect(got).toHaveLength(2)
    expect(got.map((r) => r.name)).toContain('orders')
  })

  it('reachableRefs returns just the seed when it has no edges', () => {
    expect(reachableRefs([t('isolated')], edges).map((r) => r.name)).toEqual(['isolated'])
  })

  it('reachableRefs honors maxDepth (1 hop vs 2 hops)', () => {
    const oneHop = reachableRefs([t('orders')], edges, 60, 1)
      .map((r) => r.name)
      .sort()
    expect(oneHop).toEqual(['orders', 'products', 'users']) // reviews is 2 hops away via products
    const twoHop = reachableRefs([t('orders')], edges, 60, 2)
      .map((r) => r.name)
      .sort()
    expect(twoHop).toEqual(['orders', 'products', 'reviews', 'users'])
  })

  it('planNamespaceSeed returns all tables when under the cap', () => {
    const refs = [t('users'), t('orders'), t('products')]
    expect(planNamespaceSeed(refs, edges, 60)).toEqual({ seed: refs, progressive: false })
  })

  it('planNamespaceSeed returns hub tables and progressive=true when over the cap', () => {
    const refs = [t('users'), t('orders'), t('products'), t('reviews')]
    const { seed, progressive } = planNamespaceSeed(refs, edges, 2)
    expect(progressive).toBe(true)
    expect(seed).toHaveLength(2)
    expect(seed.map((r) => r.name).sort()).toEqual(['orders', 'products'])
  })

  it('estimateNodeSize grows with column count and shrinks when collapsed', () => {
    const detail = {
      ref: t('users'),
      relational: { columns: [{ name: 'a', data_type: 'int', nullable: false, ordinal: 1 }] },
    } as ObjectDetail
    const open = estimateNodeSize(detail, false)
    const collapsed = estimateNodeSize(detail, true)
    expect(open.height).toBeGreaterThan(collapsed.height)
    expect(DIAGRAM_MAX_TABLES).toBe(60)
  })

  it('estimateNodeSize in keys-only mode counts only PK/FK columns (plus a row for the rest)', () => {
    const col = (name: string) => ({ name, data_type: 'int', nullable: false, ordinal: 1 })
    const detail = {
      ref: t('orders'),
      relational: {
        columns: [col('id'), col('user_id'), col('note'), col('total')],
        primary_key: ['id'],
        foreign_keys: [
          { columns: ['user_id'], references: t('users'), referenced_columns: ['id'] },
        ],
      },
    } as ObjectDetail
    const all = estimateNodeSize(detail, false)
    const keys = estimateNodeSize(detail, false, true)
    // 2 key columns (id, user_id) + 1 "more" row < 4 columns.
    expect(keys.height).toBeLessThan(all.height)
  })
})

describe('edgeCardinality', () => {
  const detail = (over: object): ObjectDetail =>
    ({ ref: t('x'), relational: { columns: [], ...over } }) as ObjectDetail
  it('is one_to_many when the FK columns are not unique on the child', () => {
    expect(edgeCardinality(['parent_id'], detail({ primary_key: ['id'], indexes: [] }))).toBe(
      'one_to_many',
    )
  })
  it('is one_to_one when the FK columns match the child primary key', () => {
    expect(edgeCardinality(['id'], detail({ primary_key: ['id'] }))).toBe('one_to_one')
  })
  it('is one_to_one when the FK columns match a unique index', () => {
    expect(
      edgeCardinality(
        ['parent_id'],
        detail({
          primary_key: ['id'],
          indexes: [{ name: 'u', columns: ['parent_id'], unique: true }],
        }),
      ),
    ).toBe('one_to_one')
  })
  it('defaults to one_to_many when the child detail is missing', () => {
    expect(edgeCardinality(['parent_id'], undefined)).toBe('one_to_many')
  })
})

describe('relationshipHandleId', () => {
  const availableColumns = new Set(['store_id'])
  const connectedColumns = new Set(['store_id'])

  it('uses the column handle only after React Flow has indexed it', () => {
    expect(
      relationshipHandleId({
        column: 'store_id',
        direction: 'in',
        handlesReady: false,
        availableColumns,
        connectedColumns,
      }),
    ).toBe('node:in')
    expect(
      relationshipHandleId({
        column: 'store_id',
        direction: 'in',
        handlesReady: true,
        availableColumns,
        connectedColumns,
      }),
    ).toBe('col:store_id:in')
  })

  it('falls back when schema and relationship metadata drift', () => {
    expect(
      relationshipHandleId({
        column: 'missing',
        direction: 'out',
        handlesReady: true,
        availableColumns,
        connectedColumns,
      }),
    ).toBe('node:out')
    expect(
      relationshipHandleId({
        column: 'store_id',
        direction: 'out',
        handlesReady: true,
        availableColumns,
        connectedColumns: new Set(),
      }),
    ).toBe('node:out')
  })
})
