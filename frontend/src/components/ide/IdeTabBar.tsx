import { HugeiconsIcon } from '@hugeicons/react'
import {
  Cancel01Icon,
  DatabaseIcon,
  File01Icon,
  TerminalIcon,
} from '@hugeicons/core-free-icons'
import type { Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, newScratchTab, type EditorTab, type TabKind } from './useIdeStore'

type IdeTabBarProps = {
  workspace: Workspace
}

const TAB_ICONS: Record<TabKind, typeof DatabaseIcon> = {
  scratch: TerminalIcon,
  file: File01Icon,
  connection: DatabaseIcon,
}

export function IdeTabBar({ workspace }: IdeTabBarProps) {
  const activeWorkspaceId = useIde((s) => s.activeWorkspaceId)
  const activeTabId = useIde((s) => s.activeTabId)
  const tabs = useIde((s) => s.tabs)
  const openTab = useIde((s) => s.openTab)
  const closeTab = useIde((s) => s.closeTab)
  const setActiveTab = useIde((s) => s.setActiveTab)

  const workspaceTabs = tabs.filter((t) => t.workspaceId === (activeWorkspaceId ?? workspace.id))

  function handleNewScratch() {
    openTab(newScratchTab(workspace))
  }

  return (
    <div className="flex h-9 shrink-0 items-end overflow-x-auto bg-background [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      {workspaceTabs.map((tab) => (
        <TabItem
          key={tab.id}
          tab={tab}
          active={tab.id === activeTabId}
          onActivate={() => setActiveTab(tab.id)}
          onClose={() => closeTab(tab.id)}
        />
      ))}
      <button
        type="button"
        onClick={handleNewScratch}
        className="flex h-8 shrink-0 items-center px-3 text-xs text-muted-foreground transition-colors hover:text-foreground"
        aria-label="New SQL console"
      >
        + New
      </button>
    </div>
  )
}

function TabItem({
  tab,
  active,
  onActivate,
  onClose,
}: {
  tab: EditorTab
  active: boolean
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
      <HugeiconsIcon icon={icon} size={13} strokeWidth={2} className="shrink-0 opacity-60" />
      <span className="min-w-0 flex-1 truncate text-xs">{tab.title}</span>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          onClose()
        }}
        aria-label={`Close ${tab.title}`}
        className={cn(
          'flex size-5 shrink-0 items-center justify-center rounded transition-colors',
          'hover:bg-muted hover:text-foreground',
          active ? 'opacity-100' : 'opacity-0 group-hover:opacity-100',
        )}
      >
        <HugeiconsIcon icon={Cancel01Icon} size={11} strokeWidth={2} />
      </button>
    </div>
  )
}
