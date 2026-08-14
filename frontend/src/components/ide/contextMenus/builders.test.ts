import { describe, it, expect } from 'vitest'
import type {
  ContextMenuItem,
  ContextMenuActionItem,
  ContextMenuSubItem,
} from '#/components/ui/context-menu'
import { buildEnvironmentMenu } from './environmentMenu'
import { buildConnectionMenu } from './connectionMenu'
import { buildNamespaceMenu, buildObjectGroupMenu } from './schemaMenu'
import { buildObjectMenu } from './objectMenu'
import { buildColumnMenu, buildIndexMenu } from './columnMenu'

const noop = () => {}

function action(items: ContextMenuItem[], id: string): ContextMenuActionItem | undefined {
  return items.find((i): i is ContextMenuActionItem => i.kind === 'action' && i.id === id)
}

function submenu(items: ContextMenuItem[], id: string): ContextMenuSubItem | undefined {
  return items.find((i): i is ContextMenuSubItem => i.kind === 'submenu' && i.id === id)
}

describe('buildEnvironmentMenu', () => {
  const base = { onCopyName: noop, onManageEnvironments: noop }
  const items = buildEnvironmentMenu({
    ...base,
    onNewConnection: noop,
    onRenameEnvironment: noop,
    onDeleteEnvironment: noop,
  })
  it('has a live copy-name action', () => {
    const it = action(items, 'copy-name')
    expect(it?.soon).toBeFalsy()
    expect(typeof it?.onSelect).toBe('function')
  })
  it('has a live manage-environments action', () => {
    const item = action(items, 'manage-environments')
    expect(item?.soon).toBeFalsy()
    expect(typeof item?.onSelect).toBe('function')
  })
  it('exposes live quick actions when their callbacks are provided', () => {
    for (const id of ['new-connection', 'rename-environment', 'delete-environment']) {
      expect(action(items, id)?.soon).toBeFalsy()
      expect(typeof action(items, id)?.onSelect).toBe('function')
    }
    expect(action(items, 'delete-environment')?.destructive).toBe(true)
  })
  it('omits quick actions when their callbacks are unavailable', () => {
    const restrictedItems = buildEnvironmentMenu(base)
    expect(action(restrictedItems, 'new-connection')).toBeUndefined()
    expect(action(restrictedItems, 'rename-environment')).toBeUndefined()
    expect(action(restrictedItems, 'delete-environment')).toBeUndefined()
  })
})

describe('buildConnectionMenu', () => {
  const base = {
    canEditConnection: true,
    canDeleteConnection: true,
    onOpen: noop,
    onOpenConsole: noop,
    onConnect: noop,
    onDisconnect: noop,
    onRefreshSchema: noop,
    onCopyName: noop,
    onManageConnections: noop,
    onEditConnection: noop,
    onDeleteConnection: noop,
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
  it('has a live edit-connection action', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true })
    const item = action(items, 'edit-connection')
    expect(item?.soon).toBeFalsy()
    expect(typeof item?.onSelect).toBe('function')
  })
  it('has a live delete-connection action', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true })
    const item = action(items, 'delete-connection')
    expect(item?.soon).toBeFalsy()
    expect(typeof item?.onSelect).toBe('function')
  })
  it('has a live manage-connections action', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true })
    const item = action(items, 'manage-connections')
    expect(item?.soon).toBeFalsy()
    expect(typeof item?.onSelect).toBe('function')
  })
  it('omits edit-connection when canEditConnection is false', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true, canEditConnection: false })
    expect(action(items, 'edit-connection')).toBeUndefined()
  })
  it('omits delete-connection when canDeleteConnection is false', () => {
    const items = buildConnectionMenu({ ...base, isConnected: true, canDeleteConnection: false })
    expect(action(items, 'delete-connection')).toBeUndefined()
  })
  it('omits both when neither permission is held', () => {
    const items = buildConnectionMenu({
      ...base,
      isConnected: true,
      canEditConnection: false,
      canDeleteConnection: false,
    })
    expect(action(items, 'edit-connection')).toBeUndefined()
    expect(action(items, 'delete-connection')).toBeUndefined()
  })
})

describe('buildNamespaceMenu / buildObjectGroupMenu', () => {
  it('scope copy + refresh are live; create/drop disabled without callbacks', () => {
    const items = buildNamespaceMenu({
      onCopyName: noop,
      onRefresh: noop,
      dropLabel: 'Drop schema',
    })
    expect(action(items, 'copy-schema-name')?.soon).toBeFalsy()
    expect(action(items, 'refresh')?.soon).toBeFalsy()
    expect(action(items, 'create-table')?.disabled).toBe(true)
    expect(action(items, 'drop-schema')?.disabled).toBe(true)
    expect(action(items, 'drop-schema')?.label).toBe('Drop schema')
    expect(action(items, 'view-schema-diagram')).toBeUndefined()
  })
  it('scope shows View schema diagram when the callback is provided', () => {
    const items = buildNamespaceMenu({
      onCopyName: noop,
      onRefresh: noop,
      onViewDiagram: noop,
      dropLabel: 'Drop schema',
    })
    expect(action(items, 'view-schema-diagram')?.label).toBe('View schema diagram')
    expect(action(items, 'view-schema-diagram')?.soon).toBeFalsy()
  })
  it('scope enables create-table/drop-schema when callbacks are provided', () => {
    const items = buildNamespaceMenu({
      onCopyName: noop,
      onRefresh: noop,
      dropLabel: 'Drop schema',
      onCreateTable: noop,
      onDropScope: noop,
    })
    expect(action(items, 'create-table')?.disabled).toBeFalsy()
    expect(action(items, 'drop-schema')?.disabled).toBeFalsy()
    expect(action(items, 'drop-schema')?.destructive).toBe(true)
  })
  it('scope surfaces disabled reasons for create-table/drop-schema', () => {
    const items = buildNamespaceMenu({
      onCopyName: noop,
      onRefresh: noop,
      dropLabel: 'Drop schema',
      createTableDisabledReason: 'This driver does not support this change.',
      dropScopeDisabledReason: 'Connect to the database to make this change.',
    })
    expect(action(items, 'create-table')?.disabledReason).toBe(
      'This driver does not support this change.',
    )
    expect(action(items, 'drop-schema')?.disabledReason).toBe(
      'Connect to the database to make this change.',
    )
  })
  it('object-group omits new-object entirely without create-table support (non-table kinds)', () => {
    for (const newLabel of ['New View…', 'New Function…', 'New Sequence…', 'New Trigger…']) {
      const items = buildObjectGroupMenu({ newLabel, onRefresh: noop })
      expect(action(items, 'new-object')).toBeUndefined()
      expect(items.some((i) => i.kind === 'action' && i.soon)).toBe(false)
      expect(action(items, 'refresh')?.soon).toBeFalsy()
      expect(action(items, 'view-diagram')).toBeUndefined()
    }
  })
  it('object-group shows View diagram when the callback is provided', () => {
    const items = buildObjectGroupMenu({
      newLabel: 'New Table…',
      onRefresh: noop,
      onViewDiagram: noop,
    })
    expect(action(items, 'view-diagram')?.label).toBe('View diagram')
    expect(action(items, 'view-diagram')?.soon).toBeFalsy()
  })
  it('object-group enables new-object when onCreateTable is provided', () => {
    const items = buildObjectGroupMenu({
      newLabel: 'New Table…',
      onRefresh: noop,
      onCreateTable: noop,
    })
    expect(action(items, 'new-object')?.soon).toBeFalsy()
    expect(action(items, 'new-object')?.disabled).toBeFalsy()
  })
  it('object-group shows a disabled reason instead of soon when create-table is gated', () => {
    const items = buildObjectGroupMenu({
      newLabel: 'New Table…',
      onRefresh: noop,
      createTableDisabledReason: 'You do not have permission to change this schema.',
    })
    expect(action(items, 'new-object')?.soon).toBeFalsy()
    expect(action(items, 'new-object')?.disabled).toBe(true)
    expect(action(items, 'new-object')?.disabledReason).toBe(
      'You do not have permission to change this schema.',
    )
  })
})

describe('buildObjectMenu', () => {
  const base = { onOpen: noop, onCopyName: noop, onCopyQualifiedName: noop, onCopyColumnList: noop }
  it('exposes a live Open action', () => {
    const items = buildObjectMenu({ ...base, isView: false })
    expect(action(items, 'open')?.label).toBe('Open')
    expect(action(items, 'open')?.soon).toBeFalsy()
  })
  it('omits View diagram unless the callback is provided', () => {
    expect(action(buildObjectMenu({ ...base, isView: false }), 'view-diagram')).toBeUndefined()
    const items = buildObjectMenu({ ...base, isView: false, onViewDiagram: noop })
    expect(action(items, 'view-diagram')?.label).toBe('View diagram')
    expect(action(items, 'view-diagram')?.soon).toBeFalsy()
  })
  it('table omits the view-only action; drop disabled without a callback', () => {
    const items = buildObjectMenu({ ...base, isView: false })
    expect(action(items, 'copy-qualified-name')?.soon).toBeFalsy()
    expect(action(items, 'edit-view-definition')).toBeUndefined()
    expect(action(items, 'drop')?.disabled).toBe(true)
  })
  it('view includes edit-view-definition', () => {
    const items = buildObjectMenu({ ...base, isView: true })
    expect(action(items, 'edit-view-definition')?.soon).toBe(true)
  })
  it('drop is enabled and destructive when a callback is provided', () => {
    const items = buildObjectMenu({ ...base, isView: false, onDrop: noop })
    expect(action(items, 'drop')?.disabled).toBeFalsy()
    expect(action(items, 'drop')?.destructive).toBe(true)
  })
  it('surfaces a disabled reason when drop is gated', () => {
    const items = buildObjectMenu({
      ...base,
      isView: false,
      dropDisabledReason: 'This driver does not support this change.',
    })
    expect(action(items, 'drop')?.disabledReason).toBe('This driver does not support this change.')
  })
  it('omits the Generate submenu when no operations are advertised', () => {
    const items = buildObjectMenu({ ...base, isView: false, onGenerateStatement: noop })
    expect(submenu(items, 'generate')).toBeUndefined()
  })
  it('omits the Generate submenu when no callback is provided, even if operations exist', () => {
    const items = buildObjectMenu({
      ...base,
      isView: false,
      statementOperations: ['select', 'insert'],
    })
    expect(submenu(items, 'generate')).toBeUndefined()
  })
  it('shows the Generate submenu with plain child labels in fixed order', () => {
    const items = buildObjectMenu({
      ...base,
      isView: false,
      statementOperations: ['delete', 'select', 'insert', 'update'],
      onGenerateStatement: noop,
    })
    const generate = submenu(items, 'generate')
    expect(generate?.label).toBe('Generate')
    expect(generate?.items.map((i) => (i.kind === 'action' ? i.id : i.kind))).toEqual([
      'generate-select',
      'generate-insert',
      'generate-update',
      'generate-delete',
    ])
    expect(generate?.items.map((i) => (i.kind === 'action' ? i.label : ''))).toEqual([
      'Select',
      'Insert',
      'Update',
      'Delete',
    ])
  })
  it('a view only gets the operations the backend advertises for it', () => {
    const items = buildObjectMenu({
      ...base,
      isView: true,
      statementOperations: ['select'],
      onGenerateStatement: noop,
    })
    const generate = submenu(items, 'generate')
    expect(generate?.items).toHaveLength(1)
    expect(action(generate?.items ?? [], 'generate-select')).toBeDefined()
  })
  it('invokes onGenerateStatement with the selected operation', () => {
    const calls: string[] = []
    const items = buildObjectMenu({
      ...base,
      isView: false,
      statementOperations: ['select', 'update'],
      onGenerateStatement: (operation) => calls.push(operation),
    })
    const generate = submenu(items, 'generate')
    action(generate?.items ?? [], 'generate-update')?.onSelect?.()
    expect(calls).toEqual(['update'])
  })
})

describe('buildColumnMenu / buildIndexMenu', () => {
  it('column copy actions are live, rename/drop disabled without callbacks', () => {
    const items = buildColumnMenu({ onCopyName: noop, onCopyQualifiedName: noop, onCopyType: noop })
    expect(action(items, 'copy-column-name')?.soon).toBeFalsy()
    expect(action(items, 'copy-qualified-name')?.soon).toBeFalsy()
    expect(action(items, 'copy-type')?.soon).toBeFalsy()
    expect(action(items, 'rename')?.disabled).toBe(true)
    expect(action(items, 'drop-column')?.disabled).toBe(true)
  })
  it('column rename/drop enabled when callbacks are provided', () => {
    const items = buildColumnMenu({
      onCopyName: noop,
      onCopyQualifiedName: noop,
      onCopyType: noop,
      onRename: noop,
      onDrop: noop,
    })
    expect(action(items, 'rename')?.disabled).toBeFalsy()
    expect(action(items, 'drop-column')?.disabled).toBeFalsy()
    expect(action(items, 'drop-column')?.destructive).toBe(true)
  })
  it('index copy is live, drop disabled without a callback', () => {
    const items = buildIndexMenu({ onCopyName: noop })
    expect(action(items, 'copy-index-name')?.soon).toBeFalsy()
    expect(action(items, 'drop-index')?.disabled).toBe(true)
  })
  it('index drop enabled when a callback is provided', () => {
    const items = buildIndexMenu({ onCopyName: noop, onDrop: noop })
    expect(action(items, 'drop-index')?.disabled).toBeFalsy()
    expect(action(items, 'drop-index')?.destructive).toBe(true)
  })
})
