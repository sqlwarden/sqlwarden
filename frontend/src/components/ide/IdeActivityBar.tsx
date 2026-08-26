import { useQuery } from '@tanstack/react-query'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { SessionResponse, Workspace } from '#/lib/api/types'
import { useIde } from './useIdeStore'
import {
  visibleActivities,
  type ActivityVisibilityContext,
  type IdeActivity,
} from './ideActivities'
import { Tip } from './schema-diagram/Tip'
import { WorkspaceSelector } from './WorkspaceSelector'

type IdeActivityBarProps = {
  orgSlug: string
  workspaces: Workspace[]
  activeWorkspace: Workspace | undefined
  onSelectWorkspace: (workspaceId: number) => void
  session: SessionResponse | undefined
  canAccessOrgSettings: boolean
}

export function IdeActivityBar({
  orgSlug,
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  session: _session,
  canAccessOrgSettings: _canAccessOrgSettings,
}: IdeActivityBarProps) {
  const activeActivityId = useIde((s) => s.activeActivityId)
  const sidebarCollapsed = useIde((s) => s.sidebarCollapsed)
  const setActiveActivity = useIde((s) => s.setActiveActivity)
  const setSidebarCollapsed = useIde((s) => s.setSidebarCollapsed)

  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const visibilityContext: ActivityVisibilityContext = {
    queryHistoryMode: runtimeSettings.data?.effective.query_history_mode ?? 'backend',
    queryFavoritesMode: runtimeSettings.data?.effective.query_favorites_mode ?? 'backend',
  }
  const activities = visibleActivities(visibilityContext)

  function handleClick(activity: IdeActivity) {
    const isActive = activity.id === activeActivityId
    if (activity.mode === 'sidebar' && isActive) {
      // Re-clicking the active sidebar activity toggles the panel.
      setSidebarCollapsed(!sidebarCollapsed)
      return
    }
    setActiveActivity(activity.id)
    if (activity.mode === 'sidebar' && sidebarCollapsed) {
      setSidebarCollapsed(false)
    }
  }

  return (
    <nav
      aria-label="Editor activities"
      className="flex w-11 shrink-0 flex-col items-center gap-1 border-r border-border bg-sidebar py-2"
    >
      {activities.map((activity) => {
        const isActive = activity.id === activeActivityId
        const expanded = isActive && !(activity.mode === 'sidebar' && sidebarCollapsed)
        return (
          <Tip key={activity.id} label={activity.label} side="right">
            <button
              type="button"
              onClick={() => handleClick(activity)}
              aria-label={activity.label}
              aria-pressed={isActive}
              className={cn(
                'relative flex size-8 items-center justify-center rounded-lg transition-colors',
                expanded
                  ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                  : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground',
              )}
            >
              {expanded ? (
                <span className="absolute inset-y-1.5 -left-1.5 w-0.5 rounded-full bg-sidebar-primary" />
              ) : null}
              <Icon name={activity.icon} size={17} />
            </button>
          </Tip>
        )
      })}
      <div className="mt-auto">
        <WorkspaceSelector
          workspaces={workspaces}
          activeWorkspace={activeWorkspace}
          onSelect={onSelectWorkspace}
        />
      </div>
    </nav>
  )
}
