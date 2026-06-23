import type { ContextMenuItem } from '#/components/ui/context-menu'

export type NamespaceMenuCtx = {
  onCopyName: () => void
  onRefresh: () => void
}

export function buildNamespaceMenu(ctx: NamespaceMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'copy-schema-name', label: 'Copy schema name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'action', id: 'refresh', label: 'Refresh', icon: 'refresh', onSelect: ctx.onRefresh },
    { kind: 'separator' },
    { kind: 'action', id: 'new-query', label: 'New query (set search_path / USE)', icon: 'terminal', soon: true },
    { kind: 'action', id: 'create-table', label: 'Create table…', icon: 'plus-sign', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'drop-schema', label: 'Drop schema', icon: 'delete-01', soon: true },
  ]
}

export type ObjectGroupMenuCtx = {
  newLabel: string
  onRefresh: () => void
}

export function buildObjectGroupMenu(ctx: ObjectGroupMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'new-object', label: ctx.newLabel, icon: 'plus-sign', soon: true },
    { kind: 'action', id: 'refresh', label: 'Refresh', icon: 'refresh', onSelect: ctx.onRefresh },
  ]
}
