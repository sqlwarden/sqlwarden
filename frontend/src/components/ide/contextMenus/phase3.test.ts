import { describe, it, expect } from 'vitest'
import type { ContextMenuItem, ContextMenuActionItem } from '#/components/ui/context-menu'
import { buildTabMenu } from './tabMenu'
import { buildCellMenu, buildRowMenu, buildColumnHeaderMenu } from './resultMenu'
import { buildWorkspaceMenu } from './workspaceMenu'

const noop = () => {}

function action(items: ContextMenuItem[], id: string): ContextMenuActionItem | undefined {
  return items.find((i): i is ContextMenuActionItem => i.kind === 'action' && i.id === id)
}

describe('buildTabMenu', () => {
  const base = {
    onClose: noop, onCloseOthers: noop, onCloseRight: noop, onCloseAll: noop,
    onSplitRight: noop, onSplitDown: noop, onCopyName: noop,
  }
  it('has live close + split and live batch-close actions', () => {
    const items = buildTabMenu({ ...base, isConsole: false, hasOthers: true, hasRight: true })
    expect(action(items, 'close')?.soon).toBeFalsy()
    expect(action(items, 'split')?.soon).toBeFalsy()
    expect(action(items, 'split-down')?.soon).toBeFalsy()
    expect(action(items, 'close-others')?.soon).toBeFalsy()
    expect(action(items, 'close-to-right')?.soon).toBeFalsy()
    expect(action(items, 'close-all')?.soon).toBeFalsy()
  })
  it('disables close-others / close-to-right when there are none', () => {
    const items = buildTabMenu({ ...base, isConsole: false, hasOthers: false, hasRight: false })
    expect(action(items, 'close-others')?.disabled).toBe(true)
    expect(action(items, 'close-to-right')?.disabled).toBe(true)
    expect(action(items, 'close-all')?.disabled).toBeFalsy()
  })
  it('adds a soon rename only for console tabs', () => {
    const ctx = { ...base, hasOthers: true, hasRight: true }
    expect(action(buildTabMenu({ ...ctx, isConsole: false }), 'rename')).toBeUndefined()
    expect(action(buildTabMenu({ ...ctx, isConsole: true }), 'rename')?.soon).toBe(true)
  })
})

describe('buildCellMenu', () => {
  const items = buildCellMenu({ onCopyValue: noop, onCopyColumnName: noop })
  it('has live copy + copy-column-name', () => {
    expect(action(items, 'copy')?.soon).toBeFalsy()
    expect(action(items, 'copy-column-name')?.soon).toBeFalsy()
  })
})

describe('buildRowMenu', () => {
  const items = buildRowMenu({ onCopyRow: noop, onCopyRowJson: noop })
  it('has live copy actions and a soon delete-row', () => {
    expect(action(items, 'copy-row')?.soon).toBeFalsy()
    expect(action(items, 'copy-row-json')?.soon).toBeFalsy()
    expect(action(items, 'delete-row')?.soon).toBe(true)
  })
})

describe('buildColumnHeaderMenu', () => {
  const items = buildColumnHeaderMenu({ onCopyName: noop, onCopyAllValues: noop })
  it('has live copy actions and soon sort', () => {
    expect(action(items, 'copy-column-name')?.soon).toBeFalsy()
    expect(action(items, 'copy-all-values')?.soon).toBeFalsy()
    expect(action(items, 'sort-asc')?.soon).toBe(true)
  })
})

describe('buildWorkspaceMenu', () => {
  const items = buildWorkspaceMenu({ onNewConsole: noop, onCopyName: noop })
  it('has live new-console + copy-name, soon settings', () => {
    expect(action(items, 'new-console')?.soon).toBeFalsy()
    expect(action(items, 'copy-workspace-name')?.soon).toBeFalsy()
    expect(action(items, 'workspace-settings')?.soon).toBe(true)
  })
})
