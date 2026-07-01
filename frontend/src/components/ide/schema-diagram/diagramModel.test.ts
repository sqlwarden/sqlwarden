import { describe, it, expect } from 'vitest'
import {
  refKey, hiddenNeighbors, rankByDegree, planObjectSeed, planNamespaceSeed, estimateNodeSize, DIAGRAM_MAX_TABLES,
} from './diagramModel'
import type { ObjectRef, Relationship, ObjectDetail } from '#/lib/api/types'

const t = (name: string): ObjectRef => ({ namespace: 'public', kind: 'table', name })
const edge = (from: string, to: string): Relationship => ({
  name: `${from}_${to}_fk`, source: t(from), columns: ['x'], references: t(to), referenced_columns: ['id'],
})
const edges: Relationship[] = [edge('orders', 'users'), edge('orders', 'products'), edge('reviews', 'products')]

describe('diagramModel', () => {
  it('refKey is stable and namespace+kind+name scoped', () => {
    expect(refKey(t('users'))).toBe('public table users')
  })

  it('hiddenNeighbors returns both directions excluding those already present', () => {
    const present = new Set([refKey(t('orders'))])
    const got = hiddenNeighbors(t('orders'), edges, present).map((r) => r.name).sort()
    expect(got).toEqual(['products', 'users'])
  })

  it('rankByDegree orders by FK-connection count desc', () => {
    const ranked = rankByDegree([t('users'), t('orders'), t('products'), t('reviews')], edges).map((r) => r.name)
    expect(ranked.slice(0, 2).sort()).toEqual(['orders', 'products'])
  })

  it('planObjectSeed returns the ref plus its 1-hop neighbors', () => {
    const seed = planObjectSeed(t('orders'), edges).map((r) => r.name).sort()
    expect(seed).toEqual(['orders', 'products', 'users'])
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
    const detail = { ref: t('users'), relational: { columns: [{ name: 'a', data_type: 'int', nullable: false, ordinal: 1 }] } } as ObjectDetail
    const open = estimateNodeSize(detail, false)
    const collapsed = estimateNodeSize(detail, true)
    expect(open.height).toBeGreaterThan(collapsed.height)
    expect(DIAGRAM_MAX_TABLES).toBe(60)
  })
})
