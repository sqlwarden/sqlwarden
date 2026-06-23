import { createContext, useCallback, useContext, useState } from 'react'
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

export type OpenContextMenu = (items: ContextMenuItem[], e: React.MouseEvent) => void

const ContextMenuContext = createContext<OpenContextMenu | null>(null)
const noopOpener: OpenContextMenu = () => {}

/**
 * Mounts a SINGLE menu + confirm dialog for the whole subtree. Every trigger
 * (tree node, file, tab, grid cell, …) opens this one menu via the opener from
 * `useContextMenuOpener`, instead of each mounting its own Base UI Menu.Root —
 * which does not scale to large trees/result sets.
 */
export function ContextMenuProvider({ children }: { children: React.ReactNode }) {
  const [target, setTarget] = useState<{ items: ContextMenuItem[]; x: number; y: number } | null>(null)
  const [open, setOpen] = useState(false)
  const [confirmItem, setConfirmItem] = useState<ContextMenuActionItem | null>(null)

  const openContextMenu = useCallback<OpenContextMenu>((items, e) => {
    e.preventDefault()
    e.stopPropagation()
    setTarget({ items, x: e.clientX, y: e.clientY })
    setOpen(true)
  }, [])

  function selectAction(item: ContextMenuActionItem) {
    const outcome = resolveMenuAction(item)
    if (outcome === 'noop') return
    setOpen(false)
    if (outcome === 'confirm') setConfirmItem(item)
    else item.onSelect?.()
  }

  const items = target?.items ?? []

  return (
    <ContextMenuContext.Provider value={openContextMenu}>
      {children}
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger
          nativeButton={false}
          render={
            <span
              style={{ position: 'fixed', left: target?.x ?? 0, top: target?.y ?? 0, width: 0, height: 0, pointerEvents: 'none' }}
            />
          }
        />
        <DropdownMenuContent align="start" side="bottom" sideOffset={2} className="w-52">
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
    </ContextMenuContext.Provider>
  )
}

/** The opener for the nearest ContextMenuProvider; a no-op when none is mounted. */
export function useContextMenuOpener(): OpenContextMenu {
  return useContext(ContextMenuContext) ?? noopOpener
}

/**
 * Thin wrapper: attaches a right-click handler to a div that opens the shared
 * provider menu with `items`. Does not mount its own menu, so it is cheap to
 * render one per node.
 */
export function ContextMenu({
  items,
  children,
  className,
}: {
  items: ContextMenuItem[]
  children: React.ReactNode
  className?: string
}) {
  const open = useContextMenuOpener()
  return (
    <div onContextMenu={(e) => open(items, e)} className={className}>
      {children}
    </div>
  )
}

function SoonBadge() {
  return (
    <span className="ml-auto rounded bg-muted px-1 text-[9px] font-medium uppercase tracking-wide text-muted-foreground">
      soon
    </span>
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
