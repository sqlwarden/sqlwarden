import type { ContextMenuItem } from '#/components/ui/context-menu'
import type {
  DbColumn,
  NavigatorFolder,
  NavigatorItem,
  NavigatorNode,
  NavigatorTree,
  ObjectRef,
  SchemaTreeResponse,
  ScopePath,
  StatementOperation,
} from '#/lib/api/types'
import { buildColumnMenu, buildIndexMenu } from '../contextMenus/columnMenu'
import { columnList, copyWithToast, qualifiedColumn } from '../contextMenus/clipboard'
import { buildObjectMenu } from '../contextMenus/objectMenu'
import { buildNamespaceMenu, buildObjectGroupMenu } from '../contextMenus/schemaMenu'
import { statementOperationsFor } from '../generateStatementCapability'
import type { DiagramTarget } from '../schema-diagram/diagramTab'
import {
  canCreateTable,
  canDropColumn,
  canDropIndex,
  canDropObject,
  canDropScope,
  canRenameColumn,
  tableOnlyGate,
  type SchemaEditGate,
} from '../schemaEditCapability'
import type { SqlDialect } from '../sqlDialect'
import { refOfPath, type NavigatorRow } from './model'

export interface NavigatorEditActions {
  createTable: (scope: ScopePath) => void
  dropScope: (scope: ScopePath, scopeKind: string) => void
  dropObject: (ref: ObjectRef) => void
  editColumn: (ref: ObjectRef, column?: DbColumn) => void
  renameColumn: (ref: ObjectRef, columnName: string) => void
  dropColumn: (ref: ObjectRef, columnName: string) => void
  createIndex: (ref: ObjectRef) => void
  dropIndex: (ref: ObjectRef, indexName: string) => void
  generateStatement: (ref: ObjectRef, operation: StatementOperation) => void
}

export interface NavigatorActions {
  tree: SchemaTreeResponse
  dialect: SqlDialect
  sessionId?: string
  canMutate: boolean
  refresh: (path: ScopePath) => void
  openObject: (ref: ObjectRef) => void
  openDiagram: (target: DiagramTarget) => void
  columnsOf: (path: ScopePath) => NavigatorItem[]
  edit: NavigatorEditActions
}

type FolderRow = Extract<NavigatorRow, { type: 'folder' }>
type ObjectRow = Extract<NavigatorRow, { type: 'object' }>
type LeafRow = Extract<NavigatorRow, { type: 'leaf' }>

const VIEW_KINDS = new Set(['view', 'materialized_view'])

export function refreshPathOf(row: NavigatorRow): ScopePath {
  switch (row.type) {
    case 'object':
      return row.item.path
    case 'leaf':
      return row.item.path.slice(0, -1)
    case 'folder':
    case 'status':
      return row.parent
  }
}

/** True when the object at `path` lives directly under a scope (or the root) rather than under another object. */
export function isObjectLevel(tree: NavigatorTree, path: ScopePath): boolean {
  if (path.length < 2) return true
  return tree.nodes[path[path.length - 2].kind]?.scope === true
}

export function scopeSupportsDiagram(tree: NavigatorTree, node: NavigatorNode): boolean {
  return node.folders.some((folder) => tree.nodes[folder.child]?.supports_diagram)
}

export function columnFolderOf(
  tree: NavigatorTree,
  node: NavigatorNode | undefined,
): NavigatorFolder | undefined {
  return node?.folders.find((folder) => tree.nodes[folder.child]?.column)
}

export function columnOf(item: NavigatorItem): DbColumn {
  const attributes = item.attributes ?? {}
  return {
    name: item.name,
    data_type: typeof attributes.data_type === 'string' ? attributes.data_type : '',
    nullable: attributes.nullable === true,
    ordinal: typeof attributes.ordinal === 'number' ? attributes.ordinal : 0,
  }
}

const reasonOf = (gate: SchemaEditGate) => (gate.allowed ? undefined : gate.reason)

const refreshItem = (onSelect: () => void): ContextMenuItem => ({
  kind: 'action',
  id: 'refresh',
  label: 'Refresh',
  icon: 'refresh',
  onSelect,
})

export function navigatorMenu(row: NavigatorRow, actions: NavigatorActions): ContextMenuItem[] {
  const refresh = () => actions.refresh(refreshPathOf(row))
  switch (row.type) {
    case 'folder':
      return folderMenu(row, actions, refresh)
    case 'object':
      return row.node.scope
        ? scopeMenu(row, actions, refresh)
        : objectMenu(row.item, row.node, actions, refresh)
    case 'leaf':
      return leafMenu(row, actions, refresh)
    case 'status':
      return [refreshItem(refresh)]
  }
}

function folderMenu(row: FolderRow, actions: NavigatorActions, refresh: () => void) {
  const child = actions.tree.nodes[row.folder.child]
  const scopeKind = row.parent[row.parent.length - 1]?.kind ?? ''
  const createGate =
    row.folder.child === 'table'
      ? canCreateTable(actions.tree.editor, actions.sessionId, actions.canMutate, scopeKind)
      : null
  return buildObjectGroupMenu({
    newLabel: `New ${child?.label ?? row.folder.label}...`,
    onRefresh: refresh,
    onViewDiagram: child?.supports_diagram
      ? () => actions.openDiagram({ kind: 'scope', scope: row.parent })
      : undefined,
    onCreateTable: createGate?.allowed ? () => actions.edit.createTable(row.parent) : undefined,
    createTableDisabledReason: createGate ? reasonOf(createGate) : undefined,
  })
}

function scopeMenu(row: ObjectRow, actions: NavigatorActions, refresh: () => void) {
  const { item, node } = row
  const createGate = canCreateTable(
    actions.tree.editor,
    actions.sessionId,
    actions.canMutate,
    item.kind,
  )
  const dropGate = canDropScope(
    actions.tree.editor,
    actions.sessionId,
    actions.canMutate,
    item.kind,
  )
  return buildNamespaceMenu({
    onCopyName: () => copyWithToast(item.name),
    onRefresh: refresh,
    onViewDiagram: scopeSupportsDiagram(actions.tree, node)
      ? () => actions.openDiagram({ kind: 'scope', scope: item.path })
      : undefined,
    dropLabel: `Drop ${node.label.toLowerCase()}`,
    onCreateTable: createGate.allowed ? () => actions.edit.createTable(item.path) : undefined,
    createTableDisabledReason: reasonOf(createGate),
    onDropScope: dropGate.allowed ? () => actions.edit.dropScope(item.path, item.kind) : undefined,
    dropScopeDisabledReason: reasonOf(dropGate),
  })
}

function objectMenu(
  item: NavigatorItem,
  node: NavigatorNode | undefined,
  actions: NavigatorActions,
  refresh: () => void,
) {
  const ref = refOfPath(item.path)
  const { editor, statements } = actions.tree
  const { dialect, sessionId, canMutate } = actions
  const dropGate = canDropObject(editor, sessionId, canMutate, ref.kind)
  const addColumnGate = tableOnlyGate(editor, sessionId, canMutate, ref.kind, 'add_column')
  const createIndexGate = tableOnlyGate(editor, sessionId, canMutate, ref.kind, 'create_index')
  return buildObjectMenu({
    isView: VIEW_KINDS.has(ref.kind),
    onOpen: () => actions.openObject(ref),
    onRefresh: refresh,
    onViewDiagram: node?.supports_diagram
      ? () => actions.openDiagram({ kind: 'object', ref })
      : undefined,
    onCopyName: () => copyWithToast(item.name),
    onCopyQualifiedName: () => copyWithToast(dialect.formatObject(ref.scope, ref.name)),
    onCopyColumnList: () =>
      copyWithToast(
        columnList(actions.columnsOf(item.path).map((c) => dialect.formatColumn(c.name))),
      ),
    onDrop: dropGate.allowed ? () => actions.edit.dropObject(ref) : undefined,
    dropDisabledReason: reasonOf(dropGate),
    statementOperations: statementOperationsFor(statements, ref.kind),
    onGenerateStatement: (operation) => actions.edit.generateStatement(ref, operation),
    onAddColumn: addColumnGate.allowed ? () => actions.edit.editColumn(ref) : undefined,
    onCreateIndex: createIndexGate.allowed ? () => actions.edit.createIndex(ref) : undefined,
  })
}

function leafMenu(row: LeafRow, actions: NavigatorActions, refresh: () => void) {
  const { item } = row
  if (isObjectLevel(actions.tree, item.path)) return objectMenu(item, row.node, actions, refresh)

  const owner = refOfPath(item.path.slice(0, -1))
  const { editor } = actions.tree
  const { dialect, sessionId, canMutate } = actions
  if (row.node?.column) {
    const column = columnOf(item)
    const alterGate = tableOnlyGate(editor, sessionId, canMutate, owner.kind, 'alter_column')
    const renameGate = canRenameColumn(editor, sessionId, canMutate, owner.kind)
    const dropGate = canDropColumn(editor, sessionId, canMutate, owner.kind)
    return [
      ...buildColumnMenu({
        onCopyName: () => copyWithToast(dialect.formatColumn(item.name)),
        onCopyQualifiedName: () => copyWithToast(qualifiedColumn(dialect, owner.name, item.name)),
        onCopyType: () => copyWithToast(column.data_type),
        onAlter: alterGate.allowed ? () => actions.edit.editColumn(owner, column) : undefined,
        onRename: renameGate.allowed
          ? () => actions.edit.renameColumn(owner, item.name)
          : undefined,
        renameDisabledReason: reasonOf(renameGate),
        onDrop: dropGate.allowed ? () => actions.edit.dropColumn(owner, item.name) : undefined,
        dropDisabledReason: reasonOf(dropGate),
      }),
      { kind: 'separator' },
      refreshItem(refresh),
    ] satisfies ContextMenuItem[]
  }
  if (item.kind === 'index') {
    const dropGate = canDropIndex(editor, sessionId, canMutate, owner.kind)
    return [
      ...buildIndexMenu({
        onCopyName: () => copyWithToast(item.name),
        onDrop: dropGate.allowed ? () => actions.edit.dropIndex(owner, item.name) : undefined,
        dropDisabledReason: reasonOf(dropGate),
      }),
      { kind: 'separator' },
      refreshItem(refresh),
    ] satisfies ContextMenuItem[]
  }
  return [
    {
      kind: 'action',
      id: 'copy-name',
      label: 'Copy name',
      icon: 'copy-01',
      onSelect: () => copyWithToast(item.name),
    },
    refreshItem(refresh),
  ] satisfies ContextMenuItem[]
}
