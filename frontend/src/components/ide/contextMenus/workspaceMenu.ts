import type { ContextMenuItem } from '#/components/ui/context-menu'

export type WorkspaceMenuCtx = {
  onNewConsole: () => void
  onCopyName: () => void
}

export function buildWorkspaceMenu(ctx: WorkspaceMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'new-console', label: 'New console', icon: 'terminal', onSelect: ctx.onNewConsole },
    { kind: 'action', id: 'new-file', label: 'New file', icon: 'file-01', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'copy-workspace-name', label: 'Copy workspace name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'separator' },
    { kind: 'action', id: 'workspace-settings', label: 'Workspace settings', icon: 'settings-02', soon: true },
    { kind: 'action', id: 'manage-members', label: 'Manage members', soon: true },
  ]
}
