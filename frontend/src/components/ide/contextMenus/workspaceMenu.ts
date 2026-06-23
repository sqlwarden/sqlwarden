import type { ContextMenuItem } from '#/components/ui/context-menu'

export type WorkspaceMenuCtx = {
  onOpenSettings: () => void
  onManageMembers: () => void
  onManageAccess: () => void
}

export function buildWorkspaceMenu(ctx: WorkspaceMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'workspace-settings', label: 'Workspace settings', icon: 'settings-02', onSelect: ctx.onOpenSettings },
    { kind: 'action', id: 'manage-members', label: 'Manage members', icon: 'user-multiple', onSelect: ctx.onManageMembers },
    { kind: 'action', id: 'manage-access', label: 'Manage access', icon: 'shield-user', onSelect: ctx.onManageAccess },
  ]
}
