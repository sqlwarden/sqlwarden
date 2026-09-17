import { useQuery } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { Workspace } from '#/lib/api/types'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import { useIde } from './useIdeStore'
import { visibleBottomPanels } from './bottomPanels'
import type { ActivityVisibilityContext } from './ideActivities'

type BottomPanelProps = {
  orgSlug: string
  workspace: Workspace
}

function useVisibleBottomPanels(orgSlug: string) {
  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const visibilityContext: ActivityVisibilityContext = {
    queryHistoryMode: runtimeSettings.data?.effective.query_history_mode ?? 'backend',
    queryFavoritesMode: runtimeSettings.data?.effective.query_favorites_mode ?? 'backend',
  }
  return visibleBottomPanels(visibilityContext)
}

/** The persistent bottom bar — always visible while the IDE is expanded.
 *  Clicking a tab opens it as the bottom panel; clicking the open tab again
 *  closes the panel, leaving the bar itself in place. */
export function BottomPanelBar({ orgSlug }: Pick<BottomPanelProps, 'orgSlug'>) {
  const activeBottomPanelId = useIde((s) => s.activeBottomPanelId)
  const setActiveBottomPanel = useIde((s) => s.setActiveBottomPanel)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const panels = useVisibleBottomPanels(orgSlug)

  return (
    <div className="flex h-8 shrink-0 items-center gap-0.5 border-t border-border bg-sidebar px-1">
      {panels.map((panel) => {
        const isOpen = panel.id === activeBottomPanelId
        return (
          <button
            key={panel.id}
            type="button"
            onClick={() => {
              if (isOpen) {
                setActiveBottomPanel(null)
                if (maximizedPane === 'results') setMaximizedPane(null)
              } else {
                setActiveBottomPanel(panel.id)
                if (maximizedPane === 'editor') setMaximizedPane(null)
              }
            }}
            aria-pressed={isOpen}
            className={cn(
              'flex h-6 shrink-0 items-center gap-1.5 rounded-sm px-2 text-xs transition-colors',
              isOpen
                ? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
                : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground',
            )}
          >
            <Icon name={panel.icon} size={13} className="shrink-0" />
            {panel.label}
          </button>
        )
      })}
    </div>
  )
}

/** The bottom panel's content: whichever tab is open in the bar, rendered
 *  with maximize/close controls threaded into its own first content row
 *  rather than a generic wrapper header. Renders nothing (and its parent
 *  `ResizablePanel` collapses to 0) when no tab is open. */
export function BottomPanelContent({ orgSlug, workspace }: BottomPanelProps) {
  const activeBottomPanelId = useIde((s) => s.activeBottomPanelId)
  const setActiveBottomPanel = useIde((s) => s.setActiveBottomPanel)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const panels = useVisibleBottomPanels(orgSlug)
  const activePanel = panels.find((p) => p.id === activeBottomPanelId)

  if (!activePanel) return null
  const Content = activePanel.component
  const isMaximized = maximizedPane === 'results'

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <Content
        orgSlug={orgSlug}
        workspace={workspace}
        isMaximized={isMaximized}
        onMaximize={() => setMaximizedPane(isMaximized ? null : 'results')}
        onClose={() => {
          setActiveBottomPanel(null)
          if (isMaximized) setMaximizedPane(null)
        }}
      />
    </div>
  )
}
