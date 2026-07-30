import { describe, it, expect } from 'vitest'
import { exportDimensions, diagramFileName } from './export'

describe('exportDimensions', () => {
  it('scales the bounds by the resolution factor', () => {
    expect(exportDimensions({ width: 400, height: 300 }, 2)).toEqual({ width: 800, height: 600 })
  })
  it('preserves aspect ratio when clamping to the max side', () => {
    const { width, height } = exportDimensions({ width: 4000, height: 2000 }, 2, 4096)
    expect(Math.max(width, height)).toBe(4096)
    expect(width / height).toBeCloseTo(2, 5)
  })
  it('never returns zero for an empty/degenerate bounds', () => {
    const { width, height } = exportDimensions({ width: 0, height: 0 }, 2)
    expect(width).toBeGreaterThan(0)
    expect(height).toBeGreaterThan(0)
  })
})

describe('diagramFileName', () => {
  it('names an object diagram with scope and table', () => {
    expect(diagramFileName('public', 'users', 'png')).toBe('public-users-diagram.png')
  })
  it('names a scope diagram with just the scope', () => {
    expect(diagramFileName('sales', undefined, 'svg')).toBe('sales-diagram.svg')
  })
  it('sanitizes filename-unsafe characters', () => {
    expect(diagramFileName('my schema/x', 'a b', 'png')).toBe('my_schema_x-a_b-diagram.png')
  })
  it('falls back to "schema" when the name is empty', () => {
    expect(diagramFileName('', undefined, 'png')).toBe('schema-diagram.png')
  })
})
