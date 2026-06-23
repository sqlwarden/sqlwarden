import { describe, it, expect } from 'vitest'
import { readConnectionLayout } from './useConnectionLayout'

describe('readConnectionLayout', () => {
  it('returns grouped only for the exact stored value', () => {
    expect(readConnectionLayout('grouped')).toBe('grouped')
  })
  it('defaults to flat for flat / null / anything else', () => {
    expect(readConnectionLayout('flat')).toBe('flat')
    expect(readConnectionLayout(null)).toBe('flat')
    expect(readConnectionLayout('garbage')).toBe('flat')
  })
})
