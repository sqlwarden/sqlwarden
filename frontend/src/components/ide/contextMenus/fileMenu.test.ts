import { describe, it, expect } from 'vitest'
import type { ContextMenuItem, ContextMenuActionItem } from '#/components/ui/context-menu'
import { buildFileMenu, buildFolderMenu, buildRootMenu } from './fileMenu'

const noop = () => {}

function action(items: ContextMenuItem[], id: string): ContextMenuActionItem | undefined {
  return items.find((i): i is ContextMenuActionItem => i.kind === 'action' && i.id === id)
}

describe('buildFileMenu', () => {
  const base = { name: 'query.sql', onOpen: noop, onOpenToSide: noop, onCopyName: noop, onSaveAs: noop }
  it('has live open + copy-name and no copy-path', () => {
    const items = buildFileMenu(base)
    expect(action(items, 'open')?.soon).toBeFalsy()
    expect(action(items, 'copy-name')?.soon).toBeFalsy()
    expect(action(items, 'copy-path')).toBeUndefined()
  })
  it('has live open-to-side and save-as actions', () => {
    const items = buildFileMenu(base)
    expect(action(items, 'open-to-side')?.soon).toBeFalsy()
    expect(typeof action(items, 'open-to-side')?.onSelect).toBe('function')
    expect(action(items, 'save-as')?.soon).toBeFalsy()
    expect(typeof action(items, 'save-as')?.onSelect).toBe('function')
  })
  it('includes a confirming destructive delete when onDelete is provided', () => {
    const items = buildFileMenu({ ...base, onDelete: noop })
    const del = action(items, 'delete')
    expect(del?.destructive).toBe(true)
    expect(del?.confirm?.description).toContain('query.sql')
    expect(typeof del?.onSelect).toBe('function')
  })
  it('omits delete when onDelete is absent', () => {
    expect(action(buildFileMenu(base), 'delete')).toBeUndefined()
  })
})

describe('buildFolderMenu', () => {
  const base = { name: 'reports', onCreateFile: noop, onCreateFolder: noop, onCopyName: noop }
  it('has live new-file / new-folder and soon rename', () => {
    const items = buildFolderMenu(base)
    expect(action(items, 'new-file')?.soon).toBeFalsy()
    expect(action(items, 'new-folder')?.soon).toBeFalsy()
    expect(action(items, 'rename')?.soon).toBe(true)
  })
  it('delete confirm mentions contents and is present only with onDelete', () => {
    expect(action(buildFolderMenu(base), 'delete')).toBeUndefined()
    const withDelete = buildFolderMenu({ ...base, onDelete: noop })
    expect(action(withDelete, 'delete')?.confirm?.description).toContain('all its contents')
  })
})

describe('buildRootMenu', () => {
  it('exposes live new-file, new-folder, refresh', () => {
    const items = buildRootMenu({ onCreateFile: noop, onCreateFolder: noop, onRefresh: noop })
    expect(action(items, 'new-file')?.soon).toBeFalsy()
    expect(action(items, 'new-folder')?.soon).toBeFalsy()
    expect(action(items, 'refresh')?.soon).toBeFalsy()
  })
})
