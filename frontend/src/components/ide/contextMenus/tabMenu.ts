import type { ContextMenuItem } from '#/components/ui/context-menu'

export type TabMenuCtx = {
  isConsole: boolean
  hasOthers: boolean
  hasRight: boolean
  onClose: () => void
  onCloseOthers: () => void
  onCloseRight: () => void
  onCloseAll: () => void
  onSplitRight: () => void
  onSplitDown: () => void
  onCopyName: () => void
}

export function buildTabMenu(ctx: TabMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'close', label: 'Close', icon: 'cancel-01', onSelect: ctx.onClose },
    { kind: 'action', id: 'close-others', label: 'Close others', disabled: !ctx.hasOthers, onSelect: ctx.onCloseOthers },
    { kind: 'action', id: 'close-to-right', label: 'Close to the right', disabled: !ctx.hasRight, onSelect: ctx.onCloseRight },
    { kind: 'action', id: 'close-all', label: 'Close all', onSelect: ctx.onCloseAll },
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
