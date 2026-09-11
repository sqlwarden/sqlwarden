import type { ContextMenuItem } from '#/components/ui/context-menu'
import type { CellMenuCtx, ColumnHeaderMenuCtx, RowMenuCtx } from '../contextMenus/resultMenu'

/** Copy-only context menu used by grid call sites that don't pass their own
 *  builder (e.g. CSV, object-detail preview) — no query-specific "soon" items. */
export function defaultCellMenu(ctx: CellMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy', label: 'Copy', icon: 'copy-01', onSelect: ctx.onCopyValue },
    {
      kind: 'action',
      id: 'copy-column-name',
      label: 'Copy column name',
      icon: 'copy-01',
      onSelect: ctx.onCopyColumnName,
    },
  ]
}

export function defaultRowMenu(ctx: RowMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy-row', label: 'Copy row', icon: 'copy-01', onSelect: ctx.onCopyRow },
    {
      kind: 'action',
      id: 'copy-row-json',
      label: 'Copy row as JSON',
      icon: 'copy-01',
      onSelect: ctx.onCopyRowJson,
    },
  ]
}

export function defaultColumnHeaderMenu(ctx: ColumnHeaderMenuCtx): ContextMenuItem[] {
  return [
    {
      kind: 'action',
      id: 'copy-column-name',
      label: 'Copy column name',
      icon: 'copy-01',
      onSelect: ctx.onCopyName,
    },
    {
      kind: 'action',
      id: 'copy-all-values',
      label: 'Copy all values',
      icon: 'copy-01',
      onSelect: ctx.onCopyAllValues,
    },
  ]
}
