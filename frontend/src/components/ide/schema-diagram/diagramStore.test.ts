import { describe, it, expect } from 'vitest'
import { serializeDiagram, deserializeDiagram, type DiagramPersistedState } from './diagramStore'

const state: DiagramPersistedState = {
  present: ['public table users'],
  positions: { 'public table users': { x: 10, y: 20 } },
  collapsed: [],
}

describe('diagram persistence', () => {
  it('round-trips through serialize/deserialize', () => {
    expect(deserializeDiagram(serializeDiagram(state))).toEqual(state)
  })
  it('deserializes null/garbage to an empty state', () => {
    expect(deserializeDiagram(null)).toEqual({ present: [], positions: {}, collapsed: [] })
    expect(deserializeDiagram('not json')).toEqual({ present: [], positions: {}, collapsed: [] })
  })
  it('round-trips the depth (including "all") so the working set restores on reload', () => {
    expect(deserializeDiagram(serializeDiagram({ ...state, depth: 2 })).depth).toBe(2)
    expect(deserializeDiagram(serializeDiagram({ ...state, depth: 'all' })).depth).toBe('all')
  })
  it('round-trips the camera viewport', () => {
    const vp = { x: 120, y: -40, zoom: 1.75 }
    expect(deserializeDiagram(serializeDiagram({ ...state, viewport: vp })).viewport).toEqual(vp)
  })
})
