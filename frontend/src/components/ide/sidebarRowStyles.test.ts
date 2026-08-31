import { describe, expect, it } from 'vitest'
import { sidebarActiveRowClass } from './sidebarRowStyles'

describe('sidebarActiveRowClass', () => {
  it('spans the full width with square corners when active', () => {
    const className = sidebarActiveRowClass(true)
    expect(className).toContain('w-full')
    expect(className).toContain('rounded-none')
    expect(className).not.toContain('mx-1')
  })

  it('keeps the same full-width, square chrome on hover when inactive', () => {
    const className = sidebarActiveRowClass(false)
    expect(className).toContain('w-full')
    expect(className).toContain('rounded-none')
    expect(className).not.toContain('mx-1')
  })
})
