import { useQuery } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { Workspace } from '#/lib/api/types'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import { Button } from '#/components/ui/button'
import { Tip } from './schema-diagram/Tip'
import { useIde } from './useIdeStore'
import { visibleBottomPanels } from './bottomPanels'
import type { ActivityVisibilityContext } from './ideActivities'

type BottomPanelProps = {
  orgSlug: string
  workspace: Workspace
}

/** Height of the header strip; the bottom panel collapses to exactly this so
 *  the tabs stay reachable when no panel is open. */
export const BOTTOM_PANEL_HEADER_HEIGHT = 32

function useVisibleBottomPanels(orgSlug: string) {
  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const visibilityContext: ActivityVisibilityContext = {
    queryHistoryMode: runtimeSettings.data?.effective.query_history_mode ?? 'backend',
    queryFavoritesMode: runtimeSettings.data?.effective.query_favorites_mode ?? 'backend',
  }
  return visibleBottomPanels(visibilityContext)
}

/** The bottom panel's header strip: left-aligned tab buttons with maximize/close
 *  on the right. Clicking a tab focuses it; clicking the open tab again
 *  closes the panel, leaving the strip in place. Maximize/close only render
 *  while a panel is open. */
export function BottomPanelHeader({ orgSlug }: Pick<BottomPanelProps, 'orgSlug'>) {
  const activeBottomPanelId = useIde((s) => s.activeBottomPanelId)
  const setActiveBottomPanel = useIde((s) => s.setActiveBottomPanel)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const panels = useVisibleBottomPanels(orgSlug)
  const isOpen = panels.some((p) => p.id === activeBottomPanelId)
  const isMaximized = maximizedPane === 'results'

  return (
    <div
      className="flex shrink-0 items-center bg-panel px-1"
      style={{ height: BOTTOM_PANEL_HEADER_HEIGHT }}
    >
      <div className="flex min-w-0 flex-1 items-center gap-0.5">
        {panels.map((panel) => {
          const isActive = panel.id === activeBottomPanelId
          return (
            <button
              key={panel.id}
              type="button"
              onClick={() => {
                if (isActive) {
                  setActiveBottomPanel(null)
                  if (maximizedPane === 'results') setMaximizedPane(null)
                } else {
                  setActiveBottomPanel(panel.id)
                  if (maximizedPane === 'editor') setMaximizedPane(null)
                }
              }}
              aria-pressed={isActive}
              className={cn(
                'flex h-6 shrink-0 items-center gap-1.5 rounded-sm px-2 text-xs transition-colors',
                isActive
                  ? 'bg-primary/10 font-medium text-primary'
                  : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground',
              )}
            >
              <Icon name={panel.icon} size={13} className="shrink-0" />
              {panel.label}
            </button>
          )
        })}
      </div>
      <div className="flex items-center justify-end gap-0.5">
        {isOpen ? (
          <>
            <Tip label={isMaximized ? 'Restore bottom panel' : 'Maximize bottom panel'}>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Toggle bottom panel maximize"
                onClick={() => setMaximizedPane(isMaximized ? null : 'results')}
              >
                <Icon name={isMaximized ? 'minimize' : 'maximize'} size={14} />
              </Button>
            </Tip>
            <Tip label="Close panel">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Close panel"
                onClick={() => {
                  setActiveBottomPanel(null)
                  if (isMaximized) setMaximizedPane(null)
                }}
              >
                <Icon name="cancel-01" size={14} />
              </Button>
            </Tip>
          </>
        ) : null}
      </div>
    </div>
  )
}

/** The bottom panel's content: whichever tab is open in the header. Renders
 *  nothing when no tab is open, so the parent panel collapses to the header. */
export function BottomPanelContent({ orgSlug, workspace }: BottomPanelProps) {
  const activeBottomPanelId = useIde((s) => s.activeBottomPanelId)
  const panels = useVisibleBottomPanels(orgSlug)
  const activePanel = panels.find((p) => p.id === activeBottomPanelId)

  if (!activePanel) return null
  const Content = activePanel.component

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-panel">
      <Content orgSlug={orgSlug} workspace={workspace} />
    </div>
  )
}
