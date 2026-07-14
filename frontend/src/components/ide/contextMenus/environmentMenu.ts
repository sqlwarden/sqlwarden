import type { ContextMenuItem } from '#/components/ui/context-menu'

export type EnvironmentMenuCtx = {
  onCopyName: () => void
  onManageEnvironments: () => void
}

export function buildEnvironmentMenu(ctx: EnvironmentMenuCtx): ContextMenuItem[] {
  return [
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
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'new-connection',
      label: 'New connection here',
      icon: 'plus-sign',
      soon: true,
    },
    {
      kind: 'action',
      id: 'rename-environment',
      label: 'Rename environment',
      icon: 'pencil-edit-02',
      soon: true,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'delete-environment',
      label: 'Delete environment',
      icon: 'delete-01',
      soon: true,
    },
  ]
}
