import { useState } from 'react'
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

  const workspaceTabs = tabs.filter((t) => t.workspaceId === (activeWorkspaceId ?? workspace.id))

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
      <div className="flex h-9 shrink-0 items-end overflow-x-auto bg-background [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        {workspaceTabs.map((tab) => (
          <TabItem
            key={tab.id}
            tab={tab}
            active={tab.id === activeTabId}
            isRunning={!!runningTabs[tab.id]}
            onActivate={() => setActiveTab(tab.id)}
            onClose={() => handleCloseRequest(tab)}
          />
        ))}
        <button
          type="button"
          onClick={handleNewConsole}
          className="flex h-8 shrink-0 items-center px-3 text-xs text-muted-foreground transition-colors hover:text-foreground"
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

function TabItem({
  tab,
  active,
  isRunning,
  onActivate,
  onClose,
}: {
  tab: EditorTab
  active: boolean
  isRunning: boolean
  onActivate: () => void
  onClose: () => void
}) {
  const icon = TAB_ICONS[tab.kind]

  return (
    <div
      role="tab"
      aria-selected={active}
      tabIndex={0}
      onClick={onActivate}
      onKeyDown={(e) => e.key === 'Enter' && onActivate()}
      className={cn(
        'group relative flex h-9 max-w-52 shrink-0 cursor-pointer select-none items-center gap-1 border-r border-border pl-2.5 pr-1',
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
