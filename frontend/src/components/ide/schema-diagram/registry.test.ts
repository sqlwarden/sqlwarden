import { describe, it, expect } from 'vitest'
import { getDiagramHooks } from './registry'

describe('getDiagramHooks', () => {
  it('rejects drivers without a bundled frontend implementation', () => {
    expect(() => getDiagramHooks('does-not-exist')).toThrow(
      'Unsupported frontend database driver: does-not-exist',
    )
  })
  it('returns a defined hooks object for known drivers', () => {
    expect(getDiagramHooks('postgres')).toBeDefined()
    expect(getDiagramHooks('mysql')).toBeDefined()
    expect(getDiagramHooks('sqlite')).toBeDefined()
  })
})
