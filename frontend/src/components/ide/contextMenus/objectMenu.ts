import type { ContextMenuItem } from '#/components/ui/context-menu'
import type { AppIcon } from '#/lib/icons'
import type { StatementOperation } from '#/lib/api/types'
import { STATEMENT_OPERATION_ORDER } from '../generateStatementCapability'

const STATEMENT_OPERATION_LABEL: Record<StatementOperation, string> = {
  select: 'Select',
  insert: 'Insert',
  update: 'Update',
  delete: 'Delete',
}

const STATEMENT_OPERATION_ICON: Record<StatementOperation, AppIcon> = {
  select: 'search-01',
  insert: 'plus-sign',
  update: 'pencil-edit-02',
  delete: 'delete-01',
}

export type ObjectMenuCtx = {
  isView: boolean
  onOpen: () => void
  onViewDiagram?: () => void
  onCopyName: () => void
  onCopyQualifiedName: () => void
  onCopyColumnList: () => void
  onDrop?: () => void
  dropDisabledReason?: string
  /** Operations the backend advertises for this object's kind, in display
   *  order. Empty/undefined omits the Generate submenu entirely. */
  statementOperations?: StatementOperation[]
  onGenerateStatement?: (operation: StatementOperation) => void
}

export function buildObjectMenu(ctx: ObjectMenuCtx): ContextMenuItem[] {
  const generateOperations = STATEMENT_OPERATION_ORDER.filter((operation) =>
    ctx.statementOperations?.includes(operation),
  )
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'open', label: 'Open', icon: 'arrow-up-right-01', onSelect: ctx.onOpen },
    ...(ctx.onViewDiagram
      ? [
          {
            kind: 'action',
            id: 'view-diagram',
            label: 'View diagram',
            icon: 'flow-connection',
            onSelect: ctx.onViewDiagram,
          } as ContextMenuItem,
        ]
      : []),
    ...(generateOperations.length > 0 && ctx.onGenerateStatement
      ? [
          {
            kind: 'submenu',
            id: 'generate',
            label: 'Generate',
            icon: 'sparkles',
            items: generateOperations.map((operation) => ({
              kind: 'action',
              id: `generate-${operation}`,
              label: STATEMENT_OPERATION_LABEL[operation],
              icon: STATEMENT_OPERATION_ICON[operation],
              onSelect: () => ctx.onGenerateStatement?.(operation),
            })),
          } as ContextMenuItem,
        ]
      : []),
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'copy-name',
      label: 'Copy name',
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
      id: 'copy-column-list',
      label: 'Copy column list',
      icon: 'copy-01',
      onSelect: ctx.onCopyColumnList,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'drop',
      label: 'Drop',
      icon: 'delete-01',
      destructive: true,
      disabled: !ctx.onDrop,
      disabledReason: ctx.dropDisabledReason,
      onSelect: ctx.onDrop,
    },
  ]
  if (ctx.isView) {
    items.push({
      kind: 'action',
      id: 'edit-view-definition',
      label: 'Edit view definition',
      icon: 'pencil-edit-02',
      soon: true,
    })
  }
  return items
}
