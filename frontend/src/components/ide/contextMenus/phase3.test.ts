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
  const base = { onClose: noop, onSplitRight: noop, onSplitDown: noop, onCopyName: noop }
  it('has live close + split, soon close-others', () => {
    const items = buildTabMenu({ ...base, isConsole: false })
    expect(action(items, 'close')?.soon).toBeFalsy()
    expect(action(items, 'split')?.soon).toBeFalsy()
    expect(action(items, 'split-down')?.soon).toBeFalsy()
    expect(action(items, 'close-others')?.soon).toBe(true)
  })
  it('adds a soon rename only for console tabs', () => {
    expect(action(buildTabMenu({ ...base, isConsole: false }), 'rename')).toBeUndefined()
    expect(action(buildTabMenu({ ...base, isConsole: true }), 'rename')?.soon).toBe(true)
  })
})

describe('buildCellMenu', () => {
  const items = buildCellMenu({ onCopyValue: noop, onCopyColumnName: noop })
  it('has live copy + copy-column-name', () => {
    expect(action(items, 'copy')?.soon).toBeFalsy()
    expect(action(items, 'copy-column-name')?.soon).toBeFalsy()
  })
  it('exposes a copy-as submenu', () => {
    const sub = items.find((i) => i.kind === 'submenu' && i.id === 'copy-as')
    expect(sub).toBeDefined()
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
