import type { ContextMenuItem } from '#/components/ui/context-menu'

export type CellMenuCtx = {
  onCopyValue: () => void
  onCopyColumnName: () => void
}

export function buildCellMenu(ctx: CellMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy', label: 'Copy', icon: 'copy-01', onSelect: ctx.onCopyValue },
    { kind: 'action', id: 'copy-column-name', label: 'Copy column name', icon: 'copy-01', onSelect: ctx.onCopyColumnName },
    { kind: 'separator' },
    { kind: 'action', id: 'set-null', label: 'Set NULL', soon: true },
    { kind: 'action', id: 'edit-cell', label: 'Edit cell', soon: true },
  ]
}

export type RowMenuCtx = {
  onCopyRow: () => void
  onCopyRowJson: () => void
}

export function buildRowMenu(ctx: RowMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy-row', label: 'Copy row', icon: 'copy-01', onSelect: ctx.onCopyRow },
    { kind: 'action', id: 'copy-row-json', label: 'Copy row as JSON', icon: 'copy-01', onSelect: ctx.onCopyRowJson },
    { kind: 'separator' },
    { kind: 'action', id: 'copy-row-insert', label: 'Copy row as INSERT', soon: true },
    { kind: 'action', id: 'export-rows', label: 'Export rows…', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'delete-row', label: 'Delete row', icon: 'delete-01', soon: true },
  ]
}

export type ColumnHeaderMenuCtx = {
  onCopyName: () => void
  onCopyAllValues: () => void
}

export function buildColumnHeaderMenu(ctx: ColumnHeaderMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy-column-name', label: 'Copy column name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'action', id: 'copy-all-values', label: 'Copy all values', icon: 'copy-01', onSelect: ctx.onCopyAllValues },
    { kind: 'separator' },
    { kind: 'action', id: 'sort-asc', label: 'Sort ascending', soon: true },
    { kind: 'action', id: 'sort-desc', label: 'Sort descending', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'filter-by', label: 'Filter by…', soon: true },
    { kind: 'action', id: 'hide-column', label: 'Hide column', soon: true },
  ]
}
