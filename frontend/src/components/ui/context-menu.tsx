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
  const [menuOpen, setMenuOpen] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })
  const [confirmItem, setConfirmItem] = useState<ContextMenuActionItem | null>(null)

  function handleContextMenu(e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setPos({ x: e.clientX, y: e.clientY })
    setMenuOpen(true)
  }

  function selectAction(item: ContextMenuActionItem) {
    const outcome = resolveMenuAction(item)
    if (outcome === 'noop') return
    setMenuOpen(false)
    if (outcome === 'confirm') setConfirmItem(item)
    else item.onSelect?.()
  }

  return (
    <>
      <div
        onContextMenu={handleContextMenu}
        className={cn(menuOpen && 'bg-accent text-accent-foreground', className)}
      >
        {children}
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
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
      </div>

      <AlertDialog open={confirmItem !== null} onOpenChange={(open) => { if (!open) setConfirmItem(null) }}>
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
