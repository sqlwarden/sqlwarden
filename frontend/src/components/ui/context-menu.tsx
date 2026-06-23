import { useState } from 'react'
import type { AppIcon } from '#/lib/icons'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '#/components/ui/alert-dialog'

export type ContextMenuActionItem = {
  kind: 'action'
  id: string
  label: string
  icon?: AppIcon
  shortcut?: string
  disabled?: boolean
  soon?: boolean
  destructive?: boolean
  onSelect?: () => void
  confirm?: { title: string; description: string }
}

export type ContextMenuSubItem = {
  kind: 'submenu'
  id: string
  label: string
  icon?: AppIcon
  items: ContextMenuItem[]
}

export type ContextMenuSeparatorItem = { kind: 'separator' }

export type ContextMenuItem = ContextMenuActionItem | ContextMenuSubItem | ContextMenuSeparatorItem

export type MenuActionOutcome = 'noop' | 'confirm' | 'run'

/** Pure routing decision for an action item. Soon/disabled win over everything. */
export function resolveMenuAction(item: ContextMenuActionItem): MenuActionOutcome {
  if (item.soon || item.disabled) return 'noop'
  if (item.confirm) return 'confirm'
  return 'run'
}

function SoonBadge() {
  return (
    <span className="ml-auto rounded bg-muted px-1 text-[9px] font-medium uppercase tracking-wide text-muted-foreground">
      soon
    </span>
  )
}

/** Headless context-menu controller. Attach `onContextMenu` to any element
 *  (including table cells where a wrapping div would be invalid) and render
 *  `menu` inside it; the menu portals out so its host element is irrelevant. */
export function useContextMenu(items: ContextMenuItem[], contentClassName?: string) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })
  const [confirmItem, setConfirmItem] = useState<ContextMenuActionItem | null>(null)

  function onContextMenu(e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setPos({ x: e.clientX, y: e.clientY })
    setOpen(true)
  }

  function selectAction(item: ContextMenuActionItem) {
    const outcome = resolveMenuAction(item)
    if (outcome === 'noop') return
    setOpen(false)
    if (outcome === 'confirm') setConfirmItem(item)
    else item.onSelect?.()
  }

  const menu = (
    <>
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger
          nativeButton={false}
          render={
            <span
              style={{ position: 'fixed', left: pos.x, top: pos.y, width: 0, height: 0, pointerEvents: 'none' }}
            />
          }
        />
        <DropdownMenuContent align="start" side="bottom" sideOffset={2} className={cn('w-52', contentClassName)}>
          {items.map((item, i) => renderItem(item, i, selectAction))}
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog open={confirmItem !== null} onOpenChange={(o) => { if (!o) setConfirmItem(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirmItem?.confirm?.title}</AlertDialogTitle>
            <AlertDialogDescription>{confirmItem?.confirm?.description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => { const it = confirmItem; setConfirmItem(null); it?.onSelect?.() }}
              className="bg-destructive text-white hover:bg-destructive/80"
            >
              Confirm
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )

  return { open, onContextMenu, menu }
}

export function ContextMenu({
  items,
  children,
  className,
  contentClassName,
}: {
  items: ContextMenuItem[]
  children: React.ReactNode
  className?: string
  contentClassName?: string
}) {
  const { open, onContextMenu, menu } = useContextMenu(items, contentClassName)
  return (
    <div onContextMenu={onContextMenu} className={cn(open && 'bg-accent text-accent-foreground', className)}>
      {children}
      {menu}
    </div>
  )
}

function renderItem(item: ContextMenuItem, index: number, selectAction: (item: ContextMenuActionItem) => void) {
  if (item.kind === 'separator') return <DropdownMenuSeparator key={`sep-${index}`} />

  if (item.kind === 'submenu') {
    return (
      <DropdownMenuSub key={item.id}>
        <DropdownMenuSubTrigger>
          {item.icon && <Icon name={item.icon} size={13} />}
          {item.label}
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent>
          {item.items.map((sub, i) => renderItem(sub, i, selectAction))}
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    )
  }

  const inactive = item.soon || item.disabled
  return (
    <DropdownMenuItem
      key={item.id}
      disabled={inactive}
      variant={item.destructive ? 'destructive' : 'default'}
      onClick={() => selectAction(item)}
    >
      {item.icon && <Icon name={item.icon} size={13} />}
      <span className={cn(!item.soon && 'flex-1')}>{item.label}</span>
      {item.soon ? <SoonBadge /> : item.shortcut ? <DropdownMenuShortcut>{item.shortcut}</DropdownMenuShortcut> : null}
    </DropdownMenuItem>
  )
}
