import { useCallback, useEffect, useRef, useState } from 'react'
import * as Y from 'yjs'
import { Icon, type AppIcon } from '#/lib/icons'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Button } from '#/components/ui/button'
import type { Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, DEFAULT_CONSOLE_CONTENT, type EditorTab, type TabKind } from './useIdeStore'
import { DriverBadge } from './DriverBadge'

type IdeTabBarProps = {
  orgSlug: string
  workspace: Workspace
}

const TAB_ICONS: Record<TabKind, AppIcon> = {
  scratch: 'terminal',
  file: 'file-01',
  connection: 'database',
}

export function IdeTabBar({ orgSlug: _orgSlug, workspace }: IdeTabBarProps) {
  const [pendingCloseTabId, setPendingCloseTabId] = useState<string | null>(null)

  const activeWorkspaceId = useIde((s) => s.activeWorkspaceId)
  const activeTabId = useIde((s) => s.activeTabIds[s.activeWorkspaceId ?? workspace.id])
  const tabs = useIde((s) => s.tabs)
  const runningTabs = useIde((s) => s.runningTabs)
  const openConsole = useIde((s) => s.openConsole)
  const closeTab = useIde((s) => s.closeTab)
  const setActiveTab = useIde((s) => s.setActiveTab)
  const moveTab = useIde((s) => s.moveTab)

  const [draggedId, setDraggedId] = useState<string | null>(null)

  const workspaceTabs = tabs.filter((t) => t.workspaceId === (activeWorkspaceId ?? workspace.id))

  const scrollRef = useRef<HTMLDivElement>(null)
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)

  const updateScrollButtons = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    setCanScrollLeft(el.scrollLeft > 1)
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1)
  }, [])

  // Track overflow as the strip scrolls or the bar resizes.
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    updateScrollButtons()
    el.addEventListener('scroll', updateScrollButtons, { passive: true })
    const ro = new ResizeObserver(updateScrollButtons)
    ro.observe(el)
    return () => {
      el.removeEventListener('scroll', updateScrollButtons)
      ro.disconnect()
    }
  }, [updateScrollButtons])

  // Keep the active tab visible (e.g. after opening a new tab that lands off-screen).
  useEffect(() => {
    const el = scrollRef.current
    if (el && activeTabId) {
      el.querySelector(`[data-tab-id="${activeTabId}"]`)?.scrollIntoView({ inline: 'nearest', block: 'nearest' })
    }
    updateScrollButtons()
  }, [activeTabId, workspaceTabs.length, updateScrollButtons])

  function scrollTabs(direction: -1 | 1) {
    const el = scrollRef.current
    if (!el) return
    el.scrollBy({ left: direction * Math.max(160, el.clientWidth * 0.7), behavior: 'smooth' })
  }

  function handleNewConsole() {
    const tmpDoc = new Y.Doc()
    tmpDoc.getText('content').insert(0, DEFAULT_CONSOLE_CONTENT)
    const yState = Array.from(Y.encodeStateAsUpdate(tmpDoc))
    tmpDoc.destroy()
    openConsole(workspace, yState)
  }

  function handleCloseRequest(tab: EditorTab) {
    const hasConsoleContent = tab.kind === 'scratch' && tab.content.trim() !== ''
    if (runningTabs[tab.id] || (tab.kind === 'file' && tab.isDirty) || hasConsoleContent) {
      setPendingCloseTabId(tab.id)
    } else {
      closeTab(tab.id)
    }
  }

  const pendingCloseTab = pendingCloseTabId ? tabs.find((t) => t.id === pendingCloseTabId) : null
  const pendingCloseRunning = pendingCloseTab ? !!runningTabs[pendingCloseTab.id] : false
  const pendingCloseIsConsole = !pendingCloseRunning && pendingCloseTab?.kind === 'scratch'

  return (
    <>
      <div className="flex h-9 shrink-0 items-end bg-background">
        {canScrollLeft && <ScrollChevron direction="left" onClick={() => scrollTabs(-1)} />}
        <div
          ref={scrollRef}
          className="flex min-w-0 items-end overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        >
          {workspaceTabs.map((tab) => (
            <TabItem
              key={tab.id}
              tab={tab}
              active={tab.id === activeTabId}
              isRunning={!!runningTabs[tab.id]}
              dragging={draggedId === tab.id}
              onActivate={() => setActiveTab(tab.id)}
              onClose={() => handleCloseRequest(tab)}
              onDragStart={() => setDraggedId(tab.id)}
              onDragOverReorder={(position) => {
                if (draggedId && draggedId !== tab.id) moveTab(draggedId, tab.id, position)
              }}
              onDragEnd={() => setDraggedId(null)}
            />
          ))}
        </div>
        {canScrollRight && <ScrollChevron direction="right" onClick={() => scrollTabs(1)} />}
        <button
          type="button"
          onClick={handleNewConsole}
          className="flex h-9 shrink-0 items-center border-l border-border px-3 text-xs text-muted-foreground transition-colors hover:bg-card/50 hover:text-foreground"
          aria-label="New SQL console"
        >
          + New
        </button>
      </div>

      {pendingCloseTab && (
        <Dialog open={true} onOpenChange={(open) => { if (!open) setPendingCloseTabId(null) }}>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle>
                {pendingCloseRunning
                  ? 'Query still running'
                  : pendingCloseIsConsole
                  ? 'Close console?'
                  : 'Close without saving?'}
              </DialogTitle>
            </DialogHeader>
            <p className="text-sm text-muted-foreground">
              {pendingCloseRunning
                ? <>A query is still running in <strong className="text-foreground">{pendingCloseTab.title}</strong>. Closing will cancel the request.</>
                : pendingCloseIsConsole
                ? <>Content of <strong className="text-foreground">{pendingCloseTab.title}</strong> is not saved and will be permanently lost.</>
                : <>Unsaved changes to <strong className="text-foreground">{pendingCloseTab.title}</strong> will be lost.</>}
            </p>
            <DialogFooter>
              <Button variant="outline" onClick={() => setPendingCloseTabId(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => {
                  closeTab(pendingCloseTabId!)
                  setPendingCloseTabId(null)
                }}
              >
                Close anyway
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  )
}

function ScrollChevron({ direction, onClick }: { direction: 'left' | 'right'; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={direction === 'left' ? 'Scroll tabs left' : 'Scroll tabs right'}
      className={cn(
        'flex h-9 w-7 shrink-0 items-center justify-center bg-background text-muted-foreground transition-colors hover:bg-card/50 hover:text-foreground',
        direction === 'left' ? 'border-r border-border' : 'border-l border-border',
      )}
    >
      <Icon name="chevron-right" size={14} className={direction === 'left' ? 'rotate-180' : undefined} />
    </button>
  )
}

function TabItem({
  tab,
  active,
  isRunning,
  dragging,
  onActivate,
  onClose,
  onDragStart,
  onDragOverReorder,
  onDragEnd,
}: {
  tab: EditorTab
  active: boolean
  isRunning: boolean
  dragging: boolean
  onActivate: () => void
  onClose: () => void
  onDragStart: () => void
  onDragOverReorder: (position: 'before' | 'after') => void
  onDragEnd: () => void
}) {
  const icon = TAB_ICONS[tab.kind]

  return (
    <div
      role="tab"
      data-tab-id={tab.id}
      aria-selected={active}
      tabIndex={0}
      draggable
      onClick={onActivate}
      onKeyDown={(e) => e.key === 'Enter' && onActivate()}
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = 'move'
        e.dataTransfer.setData('text/plain', tab.id)
        onDragStart()
      }}
      onDragOver={(e) => {
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        const rect = e.currentTarget.getBoundingClientRect()
        onDragOverReorder(e.clientX > rect.left + rect.width / 2 ? 'after' : 'before')
      }}
      onDragEnd={onDragEnd}
      className={cn(
        'group relative flex h-9 max-w-52 shrink-0 cursor-pointer select-none items-center gap-1 border-r border-border pl-2.5 pr-1',
        dragging && 'opacity-50',
        active
          ? 'bg-card text-foreground after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[2px] after:bg-primary'
          : 'bg-background text-muted-foreground hover:bg-card/50 hover:text-foreground',
      )}
    >
      {isRunning ? (
        <Icon name="loading-03" size={13} className="shrink-0 animate-spin text-primary" />
      ) : (tab.kind === 'connection' || tab.kind === 'scratch') && tab.driver ? (
        <DriverBadge driver={tab.driver} size="sm" className="shrink-0 opacity-70" />
      ) : (
        <Icon name={icon} size={13} className="shrink-0 opacity-60" />
      )}
      <span className="min-w-0 flex-1 truncate text-xs">
        {tab.isDirty && <span className="mr-1 text-primary">●</span>}
        {tab.title}
      </span>
      <button
        type="button"
        onClick={(e) => { e.stopPropagation(); onClose() }}
        aria-label={`Close ${tab.title}`}
        className={cn(
          'flex size-5 shrink-0 items-center justify-center rounded transition-colors',
          'hover:bg-muted hover:text-foreground',
          active ? 'opacity-100' : 'opacity-0 group-hover:opacity-100',
        )}
      >
        <Icon name="cancel-01" size={11} />
      </button>
    </div>
  )
}
