import type { ContextMenuItem } from '#/components/ui/context-menu'

export type EnvironmentMenuCtx = {
  onCopyName: () => void
  onManageEnvironments: () => void
  onNewConnection?: () => void
  onRenameEnvironment?: () => void
  onDeleteEnvironment?: () => void
}

export function buildEnvironmentMenu(ctx: EnvironmentMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    {
      kind: 'action',
      id: 'copy-name',
      label: 'Copy name',
      icon: 'copy-01',
      onSelect: ctx.onCopyName,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'manage-environments',
      label: 'Manage environments',
      icon: 'settings-02',
      onSelect: ctx.onManageEnvironments,
    },
  ]

  if (ctx.onNewConnection || ctx.onRenameEnvironment) {
    items.push({ kind: 'separator' })
  }
  if (ctx.onNewConnection) {
    items.push({
      kind: 'action',
      id: 'new-connection',
      label: 'New connection here',
      icon: 'plus-sign',
      onSelect: ctx.onNewConnection,
    })
  }
  if (ctx.onRenameEnvironment) {
    items.push({
      kind: 'action',
      id: 'rename-environment',
      label: 'Rename environment',
      icon: 'pencil-edit-02',
      onSelect: ctx.onRenameEnvironment,
    })
  }
  if (ctx.onDeleteEnvironment) {
    items.push(
      { kind: 'separator' },
      {
        kind: 'action',
        id: 'delete-environment',
        label: 'Delete environment',
        icon: 'delete-01',
        destructive: true,
        onSelect: ctx.onDeleteEnvironment,
      },
    )
  }

  return items
}
