import { describe, it, expect } from 'vitest'
import { resolveMenuAction, type ContextMenuActionItem } from './context-menu'

const action = (over: Partial<ContextMenuActionItem> = {}): ContextMenuActionItem => ({
  kind: 'action',
  id: 'x',
  label: 'X',
  ...over,
})

describe('resolveMenuAction', () => {
  it('returns noop for soon items', () => {
    expect(resolveMenuAction(action({ soon: true }))).toBe('noop')
  })
  it('returns noop for disabled items', () => {
    expect(resolveMenuAction(action({ disabled: true }))).toBe('noop')
  })
  it('returns confirm when a confirm prompt is set on an enabled item', () => {
    expect(resolveMenuAction(action({ confirm: { title: 'T', description: 'D' } }))).toBe('confirm')
  })
  it('returns run for a plain enabled item', () => {
    expect(resolveMenuAction(action())).toBe('run')
  })
  it('prefers noop over confirm when a confirm item is also disabled', () => {
    expect(resolveMenuAction(action({ disabled: true, confirm: { title: 'T', description: 'D' } }))).toBe('noop')
  })
})
