import type { ContextMenuItem } from '#/components/ui/context-menu'

export type TabMenuCtx = {
  isConsole: boolean
  onClose: () => void
  onSplitRight: () => void
  onSplitDown: () => void
  onCopyName: () => void
}

export function buildTabMenu(ctx: TabMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'close', label: 'Close', icon: 'cancel-01', onSelect: ctx.onClose },
    { kind: 'action', id: 'close-others', label: 'Close others', soon: true },
    { kind: 'action', id: 'close-to-right', label: 'Close to the right', soon: true },
    { kind: 'action', id: 'close-all', label: 'Close all', soon: true },
    { kind: 'separator' },
    { kind: 'action', id: 'split', label: 'Split', onSelect: ctx.onSplitRight },
    { kind: 'action', id: 'split-down', label: 'Split down', onSelect: ctx.onSplitDown },
    { kind: 'separator' },
    { kind: 'action', id: 'pin', label: 'Pin', soon: true },
    { kind: 'action', id: 'copy-name', label: 'Copy name', icon: 'copy-01', onSelect: ctx.onCopyName },
  ]
  if (ctx.isConsole) {
    items.push({ kind: 'action', id: 'rename', label: 'Rename', icon: 'pencil-edit-02', soon: true })
  }
  return items
}
