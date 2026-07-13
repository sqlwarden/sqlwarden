import { describe, expect, it } from 'vitest'
import type { EditorTab } from './useIdeStore'
import { requiresCloseConfirmation } from './IdeTabBar'

function tab(overrides: Partial<EditorTab> = {}): EditorTab {
  return {
    id: 'scratch:1',
    workspaceId: 3,
    title: 'Console 1',
    kind: 'scratch',
    content: '',
    ...overrides,
  }
}

describe('requiresCloseConfirmation', () => {
  it('allows clean and duplicated tab instances to close immediately', () => {
    expect(requiresCloseConfirmation(tab(), false, 1)).toBe(false)
    expect(requiresCloseConfirmation(tab({ content: 'select 1' }), false, 2)).toBe(false)
  })

  it('guards running queries, dirty files, and non-empty consoles', () => {
    expect(requiresCloseConfirmation(tab(), true, 1)).toBe(true)
    expect(requiresCloseConfirmation(tab({ kind: 'file', fileId: 7, isDirty: true }), false, 1)).toBe(true)
    expect(requiresCloseConfirmation(tab({ content: 'select 1' }), false, 1)).toBe(true)
  })
})
