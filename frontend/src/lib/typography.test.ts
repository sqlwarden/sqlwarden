import { describe, expect, it } from 'vitest'
import { sectionCaptionClass } from './typography'

describe('sectionCaptionClass', () => {
  it('is a 10px uppercase tracking-wide muted-foreground utility string', () => {
    expect(sectionCaptionClass).toContain('text-[10px]')
    expect(sectionCaptionClass).toContain('uppercase')
    expect(sectionCaptionClass).toContain('text-muted-foreground')
  })
})
