import type { ContextMenuItem } from '#/components/ui/context-menu'

export type ColumnMenuCtx = {
  onCopyName: () => void
  onCopyQualifiedName: () => void
  onCopyType: () => void
  onRename?: () => void
  renameDisabledReason?: string
  onDrop?: () => void
  dropDisabledReason?: string
}

export function buildColumnMenu(ctx: ColumnMenuCtx): ContextMenuItem[] {
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
      id: 'copy-qualified-name',
      label: 'Copy qualified name',
      icon: 'copy-01',
      onSelect: ctx.onCopyQualifiedName,
    },
    {
      kind: 'action',
      id: 'copy-type',
      label: 'Copy type',
      icon: 'copy-01',
      onSelect: ctx.onCopyType,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'rename',
      label: 'Rename',
      icon: 'pencil-edit-02',
      disabled: !ctx.onRename,
      disabledReason: ctx.renameDisabledReason,
      onSelect: ctx.onRename,
    },
    {
      kind: 'action',
      id: 'drop-column',
      label: 'Drop column',
      icon: 'delete-01',
      destructive: true,
      disabled: !ctx.onDrop,
      disabledReason: ctx.dropDisabledReason,
      onSelect: ctx.onDrop,
    },
  ]
}

export type IndexMenuCtx = {
  onCopyName: () => void
  onDrop?: () => void
  dropDisabledReason?: string
}

export function buildIndexMenu(ctx: IndexMenuCtx): ContextMenuItem[] {
  return [
    {
      kind: 'action',
      id: 'copy-index-name',
      label: 'Copy index name',
      icon: 'copy-01',
      onSelect: ctx.onCopyName,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'drop-index',
      label: 'Drop index',
      icon: 'delete-01',
      destructive: true,
      disabled: !ctx.onDrop,
      disabledReason: ctx.dropDisabledReason,
      onSelect: ctx.onDrop,
    },
  ]
}
