import type { ContextMenuItem } from '#/components/ui/context-menu'

export type ColumnMenuCtx = {
  onCopyName: () => void
  onCopyQualifiedName: () => void
  onCopyType: () => void
}

export function buildColumnMenu(ctx: ColumnMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy-column-name', label: 'Copy column name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'action', id: 'copy-qualified-name', label: 'Copy qualified name', icon: 'copy-01', onSelect: ctx.onCopyQualifiedName },
    { kind: 'action', id: 'copy-type', label: 'Copy type', icon: 'copy-01', onSelect: ctx.onCopyType },
    { kind: 'separator' },
    { kind: 'action', id: 'add-to-select', label: 'Add to SELECT', soon: true },
    { kind: 'action', id: 'add-to-where', label: 'Add to WHERE', soon: true },
    { kind: 'action', id: 'filter-by-column', label: 'Filter results by this column', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'rename', label: 'Rename', icon: 'pencil-edit-02', soon: true },
    { kind: 'action', id: 'toggle-not-null', label: 'Set / Drop NOT NULL', soon: true },
    { kind: 'action', id: 'drop-column', label: 'Drop column', icon: 'delete-01', soon: true },
  ]
}

export type IndexMenuCtx = {
  onCopyName: () => void
}

export function buildIndexMenu(ctx: IndexMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy-index-name', label: 'Copy index name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'separator' },
    { kind: 'action', id: 'drop-index', label: 'Drop index', icon: 'delete-01', soon: true },
  ]
}
