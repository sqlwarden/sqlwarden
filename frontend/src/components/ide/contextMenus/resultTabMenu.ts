import type { ContextMenuItem } from '#/components/ui/context-menu'

export type ResultTabMenuCtx = {
  pinned: boolean
  hasOthers: boolean
  hasRight: boolean
  hasLeft: boolean
  onClose: () => void
  onCloseOthers: () => void
  onCloseRight: () => void
  onCloseLeft: () => void
  onTogglePin: () => void
}

export function buildResultTabMenu(ctx: ResultTabMenuCtx): ContextMenuItem[] {
  return [
    {
      kind: 'action',
      id: 'pin',
      label: ctx.pinned ? 'Unpin tab' : 'Pin tab',
      icon: 'pin-01',
      onSelect: ctx.onTogglePin,
    },
    { kind: 'separator' },
    { kind: 'action', id: 'close', label: 'Close', icon: 'cancel-01', onSelect: ctx.onClose },
    {
      kind: 'action',
      id: 'close-others',
      label: 'Close others',
      icon: 'cancel-01',
      disabled: !ctx.hasOthers,
      onSelect: ctx.onCloseOthers,
    },
    {
      kind: 'action',
      id: 'close-to-right',
      label: 'Close to the right',
      icon: 'arrow-right-01',
      disabled: !ctx.hasRight,
      onSelect: ctx.onCloseRight,
    },
    {
      kind: 'action',
      id: 'close-to-left',
      label: 'Close to the left',
      icon: 'arrow-left-01',
      disabled: !ctx.hasLeft,
      onSelect: ctx.onCloseLeft,
    },
  ]
}
