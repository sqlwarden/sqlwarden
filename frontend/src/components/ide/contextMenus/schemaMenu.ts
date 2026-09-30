import type { ContextMenuItem } from '#/components/ui/context-menu'

export type NamespaceMenuCtx = {
  onCopyName: () => void
  onRefresh: () => void
  onViewDiagram?: () => void
  /** Label naming the scope kind, e.g. "Drop schema" / "Drop database". */
  dropLabel: string
  onCreateTable?: () => void
  createTableDisabledReason?: string
  onDropScope?: () => void
  dropScopeDisabledReason?: string
}

export function buildNamespaceMenu(ctx: NamespaceMenuCtx): ContextMenuItem[] {
  return [
    ...(ctx.onViewDiagram
      ? [
          {
            kind: 'action',
            id: 'view-schema-diagram',
            label: 'View schema diagram',
            icon: 'flow-connection',
            onSelect: ctx.onViewDiagram,
          } as ContextMenuItem,
          { kind: 'separator' } as ContextMenuItem,
        ]
      : []),
    {
      kind: 'action',
      id: 'copy-schema-name',
      label: 'Copy schema name',
      icon: 'copy-01',
      onSelect: ctx.onCopyName,
    },
    { kind: 'action', id: 'refresh', label: 'Refresh', icon: 'refresh', onSelect: ctx.onRefresh },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'new-query',
      label: 'New query (set search_path / USE)',
      icon: 'terminal',
      soon: true,
    },
    {
      kind: 'action',
      id: 'create-table',
      label: 'Create table…',
      icon: 'plus-sign',
      disabled: !ctx.onCreateTable,
      disabledReason: ctx.createTableDisabledReason,
      onSelect: ctx.onCreateTable,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'drop-schema',
      label: ctx.dropLabel,
      icon: 'delete-01',
      destructive: true,
      disabled: !ctx.onDropScope,
      disabledReason: ctx.dropScopeDisabledReason,
      onSelect: ctx.onDropScope,
    },
  ]
}

export type ObjectGroupMenuCtx = {
  newLabel: string
  onRefresh: () => void
  onViewDiagram?: () => void
  onCreateTable?: () => void
  createTableDisabledReason?: string
}

export function buildObjectGroupMenu(ctx: ObjectGroupMenuCtx): ContextMenuItem[] {
  return [
    ...(ctx.onViewDiagram
      ? [
          {
            kind: 'action',
            id: 'view-diagram',
            label: 'View diagram',
            icon: 'flow-connection',
            onSelect: ctx.onViewDiagram,
          } as ContextMenuItem,
          { kind: 'separator' } as ContextMenuItem,
        ]
      : []),
    ...(ctx.onCreateTable || ctx.createTableDisabledReason
      ? [
          {
            kind: 'action',
            id: 'new-object',
            label: ctx.newLabel,
            icon: 'plus-sign',
            disabled: !ctx.onCreateTable,
            disabledReason: ctx.createTableDisabledReason,
            onSelect: ctx.onCreateTable,
          } as ContextMenuItem,
        ]
      : []),
    { kind: 'action', id: 'refresh', label: 'Refresh', icon: 'refresh', onSelect: ctx.onRefresh },
  ]
}
