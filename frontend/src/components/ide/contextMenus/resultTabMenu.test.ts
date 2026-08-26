import { describe, it, expect } from 'vitest'
import type { ContextMenuItem, ContextMenuActionItem } from '#/components/ui/context-menu'
import { buildResultTabMenu } from './resultTabMenu'

const noop = () => {}

function action(items: ContextMenuItem[], id: string): ContextMenuActionItem | undefined {
  return items.find((i): i is ContextMenuActionItem => i.kind === 'action' && i.id === id)
}

describe('buildResultTabMenu', () => {
  const base = {
    pinned: false,
    hasOthers: true,
    hasRight: true,
    hasLeft: true,
    onClose: noop,
    onCloseOthers: noop,
    onCloseRight: noop,
    onCloseLeft: noop,
    onTogglePin: noop,
  }

  it('labels the pin action "Pin tab" when unpinned, and wires it to onTogglePin', () => {
    const items = buildResultTabMenu({ ...base, pinned: false })
    expect(action(items, 'pin')?.label).toBe('Pin tab')
    expect(action(items, 'pin')?.onSelect).toBe(base.onTogglePin)
  })

  it('labels the pin action "Unpin tab" when already pinned', () => {
    const items = buildResultTabMenu({ ...base, pinned: true })
    expect(action(items, 'pin')?.label).toBe('Unpin tab')
  })

  it('wires close and the bulk-close actions to their handlers', () => {
    const items = buildResultTabMenu(base)
    expect(action(items, 'close')?.onSelect).toBe(base.onClose)
    expect(action(items, 'close-others')?.onSelect).toBe(base.onCloseOthers)
    expect(action(items, 'close-to-right')?.onSelect).toBe(base.onCloseRight)
    expect(action(items, 'close-to-left')?.onSelect).toBe(base.onCloseLeft)
  })

  it('disables bulk-close actions when there is nothing in scope', () => {
    const items = buildResultTabMenu({
      ...base,
      hasOthers: false,
      hasRight: false,
      hasLeft: false,
    })
    expect(action(items, 'close-others')?.disabled).toBe(true)
    expect(action(items, 'close-to-right')?.disabled).toBe(true)
    expect(action(items, 'close-to-left')?.disabled).toBe(true)
  })
})
