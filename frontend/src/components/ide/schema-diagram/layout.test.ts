import { describe, it, expect } from 'vitest'
import { toElkGraph } from './layout'

describe('toElkGraph', () => {
  it('maps nodes/edges to an elk graph with layered options and sizes', () => {
    const g = toElkGraph(
      [
        { id: 'a', width: 240, height: 80 },
        { id: 'b', width: 240, height: 120 },
      ],
      [{ id: 'a-b', source: 'a', target: 'b' }],
    )
    expect(g.children?.map((c) => c.id)).toEqual(['a', 'b'])
    expect(g.children?.[0]).toMatchObject({ width: 240, height: 80 })
    expect(g.edges?.[0]).toMatchObject({ sources: ['a'], targets: ['b'] })
    expect(g.layoutOptions?.['elk.algorithm']).toBe('layered')
  })
})
