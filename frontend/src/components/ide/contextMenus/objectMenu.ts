import type { ContextMenuItem } from '#/components/ui/context-menu'

export type ObjectMenuCtx = {
  isView: boolean
  onCopyName: () => void
  onCopyQualifiedName: () => void
  onCopyColumnList: () => void
}

export function buildObjectMenu(ctx: ObjectMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'copy-name', label: 'Copy name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'action', id: 'copy-qualified-name', label: 'Copy qualified name', icon: 'copy-01', onSelect: ctx.onCopyQualifiedName },
    { kind: 'action', id: 'copy-column-list', label: 'Copy column list', icon: 'copy-01', onSelect: ctx.onCopyColumnList },
    { kind: 'separator' },
    { kind: 'action', id: 'drop', label: 'Drop', icon: 'delete-01', soon: true },
  ]
  if (ctx.isView) {
    items.push({ kind: 'action', id: 'edit-view-definition', label: 'Edit view definition', icon: 'pencil-edit-02', soon: true })
  }
  return items
}
