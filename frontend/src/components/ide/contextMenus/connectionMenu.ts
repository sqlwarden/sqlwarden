import type { ContextMenuItem } from '#/components/ui/context-menu'

export type ConnectionMenuCtx = {
  isConnected: boolean
  onOpen: () => void
  onOpenConsole: () => void
  onConnect: () => void
  onDisconnect: () => void
  onRefreshSchema: () => void
  onCopyName: () => void
}

export function buildConnectionMenu(ctx: ConnectionMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'open', label: 'Open', icon: 'flow-connection', onSelect: ctx.onOpen },
    { kind: 'action', id: 'open-console', label: 'Open console', icon: 'terminal', onSelect: ctx.onOpenConsole },
    { kind: 'separator' },
    ctx.isConnected
      ? { kind: 'action', id: 'disconnect', label: 'Disconnect', icon: 'cancel-01', destructive: true, onSelect: ctx.onDisconnect }
      : { kind: 'action', id: 'connect', label: 'Connect', icon: 'flow-connection', onSelect: ctx.onConnect },
    { kind: 'action', id: 'refresh-schema', label: 'Refresh schema', icon: 'refresh', disabled: !ctx.isConnected, onSelect: ctx.onRefreshSchema },
    { kind: 'separator' },
    { kind: 'action', id: 'copy-name', label: 'Copy name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'separator' },
    { kind: 'action', id: 'edit-connection', label: 'Edit connection', icon: 'pencil-edit-02', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'delete-connection', label: 'Delete connection', icon: 'delete-01', soon: true },
  ]
}
