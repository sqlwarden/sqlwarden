import { describe, it, expect } from 'vitest'
import { getDiagramHooks } from './registry'

describe('getDiagramHooks', () => {
  it('returns a generic (empty) hooks object for unknown drivers', () => {
    const hooks = getDiagramHooks('does-not-exist')
    expect(hooks.nodeBadges).toBeUndefined()
    expect(hooks.edgeLabel).toBeUndefined()
  })
  it('returns a defined hooks object for known drivers', () => {
    expect(getDiagramHooks('postgres')).toBeDefined()
    expect(getDiagramHooks('mysql')).toBeDefined()
    expect(getDiagramHooks('sqlite')).toBeDefined()
  })
})
