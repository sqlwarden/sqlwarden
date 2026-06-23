import { describe, it, expect } from 'vitest'
import type { ContextMenuItem, ContextMenuActionItem } from '#/components/ui/context-menu'
import { buildEnvironmentMenu } from './environmentMenu'
import { buildConnectionMenu } from './connectionMenu'
import { buildNamespaceMenu, buildObjectGroupMenu } from './schemaMenu'
import { buildObjectMenu } from './objectMenu'
import { buildColumnMenu, buildIndexMenu } from './columnMenu'

const noop = () => {}

function action(items: ContextMenuItem[], id: string): ContextMenuActionItem | undefined {
  return items.find((i): i is ContextMenuActionItem => i.kind === 'action' && i.id === id)
}

describe('buildEnvironmentMenu', () => {
  const items = buildEnvironmentMenu({ onCopyName: noop })
  it('has a live copy-name action', () => {
    const it = action(items, 'copy-name')
    expect(it?.soon).toBeFalsy()
    expect(typeof it?.onSelect).toBe('function')
  })
  it('marks delete-environment as soon', () => {
    expect(action(items, 'delete-environment')?.soon).toBe(true)
  })
})

describe('buildConnectionMenu', () => {
  const base = {
    onOpen: noop, onOpenConsole: noop, onConnect: noop, onDisconnect: noop,
    onRefreshSchema: noop, onCopyName: noop,
  }
  it('shows connect (not disconnect) and disables refresh when not connected', () => {
    const items = buildConnectionMenu({ ...base, isConnected: false })
    expect(action(items, 'connect')).toBeDefined()
    expect(action(items, 'disconnect')).toBeUndefined()
    expect(action(items, 'refresh-schema')?.disabled).toBe(true)
  })
  it('shows disconnect (not connect) and enables refresh when connected', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true })
    expect(action(items, 'disconnect')).toBeDefined()
    expect(action(items, 'connect')).toBeUndefined()
    expect(action(items, 'refresh-schema')?.disabled).toBeFalsy()
  })
  it('keeps edit-connection as soon', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true })
    expect(action(items, 'edit-connection')?.soon).toBe(true)
  })
})

describe('buildNamespaceMenu / buildObjectGroupMenu', () => {
  it('namespace copy + refresh are live', () => {
    const items = buildNamespaceMenu({ onCopyName: noop, onRefresh: noop })
    expect(action(items, 'copy-schema-name')?.soon).toBeFalsy()
    expect(action(items, 'refresh')?.soon).toBeFalsy()
    expect(action(items, 'drop-schema')?.soon).toBe(true)
  })
  it('object-group uses the provided new-label and keeps it soon', () => {
    const items = buildObjectGroupMenu({ newLabel: 'New Table…', onRefresh: noop })
    expect(action(items, 'new-object')?.label).toBe('New Table…')
    expect(action(items, 'new-object')?.soon).toBe(true)
    expect(action(items, 'refresh')?.soon).toBeFalsy()
  })
})

describe('buildObjectMenu', () => {
  const base = { onCopyName: noop, onCopyQualifiedName: noop, onCopyColumnList: noop }
  it('table omits the view-only action', () => {
    const items = buildObjectMenu({ ...base, isView: false })
    expect(action(items, 'copy-qualified-name')?.soon).toBeFalsy()
    expect(action(items, 'edit-view-definition')).toBeUndefined()
    expect(action(items, 'drop')?.soon).toBe(true)
  })
  it('view includes edit-view-definition', () => {
    const items = buildObjectMenu({ ...base, isView: true })
    expect(action(items, 'edit-view-definition')?.soon).toBe(true)
  })
})

describe('buildColumnMenu / buildIndexMenu', () => {
  it('column copy actions are live, mutations are soon', () => {
    const items = buildColumnMenu({ onCopyName: noop, onCopyQualifiedName: noop, onCopyType: noop })
    expect(action(items, 'copy-column-name')?.soon).toBeFalsy()
    expect(action(items, 'copy-qualified-name')?.soon).toBeFalsy()
    expect(action(items, 'copy-type')?.soon).toBeFalsy()
    expect(action(items, 'drop-column')?.soon).toBe(true)
  })
  it('index copy is live, drop is soon', () => {
    const items = buildIndexMenu({ onCopyName: noop })
    expect(action(items, 'copy-index-name')?.soon).toBeFalsy()
    expect(action(items, 'drop-index')?.soon).toBe(true)
  })
})
