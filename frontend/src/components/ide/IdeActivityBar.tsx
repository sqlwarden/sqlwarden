import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { useIde } from './useIdeStore'
import { visibleActivities, type IdeActivity } from './ideActivities'

export function IdeActivityBar() {
  const activeActivityId = useIde((s) => s.activeActivityId)
  const sidebarCollapsed = useIde((s) => s.sidebarCollapsed)
  const setActiveActivity = useIde((s) => s.setActiveActivity)
  const setSidebarCollapsed = useIde((s) => s.setSidebarCollapsed)
  const activities = visibleActivities()

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
      aria-label="IDE activities"
      className="flex w-10 shrink-0 flex-col items-center gap-1 border-r border-border bg-sidebar py-1.5"
    >
      {activities.map((activity) => {
        const isActive = activity.id === activeActivityId
        const expanded = isActive && !(activity.mode === 'sidebar' && sidebarCollapsed)
        return (
          <button
            key={activity.id}
            type="button"
            onClick={() => handleClick(activity)}
            aria-label={activity.label}
            title={activity.label}
            aria-pressed={isActive}
            className={cn(
              'relative flex size-8 items-center justify-center rounded-md transition-colors',
              expanded
                ? 'text-foreground'
                : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
            )}
          >
            {expanded ? (
              <span className="absolute inset-y-1 left-0 w-0.5 rounded-full bg-sidebar-primary" />
            ) : null}
            <Icon name={activity.icon} size={18} />
          </button>
        )
      })}
    </nav>
  )
}
