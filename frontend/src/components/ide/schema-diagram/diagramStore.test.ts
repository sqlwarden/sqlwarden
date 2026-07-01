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
})
