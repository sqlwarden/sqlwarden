import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import { useBrand } from '#/lib/brand/brand'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { AppShellPreferencesPopover, useAppShellPreferences } from '#/components/app-shell'
import { UserAvatar } from '#/components/UserAvatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { api } from '#/lib/api/client'
import { clearAccessToken } from '#/lib/auth/access-token'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import type { SessionResponse, Workspace } from '#/lib/api/types'
import { buildUserMenuItems } from '#/lib/user-menu'
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
  onSelectWorkspace: (id: number) => void
  session: SessionResponse | undefined
  canAccessOrgSettings: boolean
}

export function IdeActivityBar({
  orgSlug,
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  session,
  canAccessOrgSettings,
}: IdeActivityBarProps) {
  const activeActivityId = useIde((s) => s.activeActivityId)
  const sidebarCollapsed = useIde((s) => s.sidebarCollapsed)
  const activityBarExpanded = useIde((s) => s.activityBarExpanded)
  const setActiveActivity = useIde((s) => s.setActiveActivity)
  const setSidebarCollapsed = useIde((s) => s.setSidebarCollapsed)
  const setActivityBarExpanded = useIde((s) => s.setActivityBarExpanded)

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
      className={cn(
        'flex shrink-0 flex-col gap-1 border-r border-border bg-sidebar py-2 transition-[width] duration-150',
        activityBarExpanded ? 'w-56 items-stretch px-2' : 'w-11 items-center',
      )}
    >
      <IdeBrand expanded={activityBarExpanded} />

      {activities.map((activity) => {
        const isActive = activity.id === activeActivityId
        const expanded = isActive && !(activity.mode === 'sidebar' && sidebarCollapsed)
        const button = (
          <button
            type="button"
            onClick={() => handleClick(activity)}
            aria-label={activity.label}
            aria-pressed={isActive}
            className={cn(
              'relative flex items-center rounded-[calc(var(--radius-sm)+2px)] text-xs transition-colors',
              activityBarExpanded ? 'h-8 w-full justify-start gap-2 p-2' : 'size-8 justify-center',
              expanded
                ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground',
            )}
          >
            {expanded ? (
              <span
                className={cn(
                  'absolute w-0.5 rounded-full bg-sidebar-primary',
                  activityBarExpanded ? 'inset-y-1.5 left-0' : 'inset-y-1.5 -left-1.5',
                )}
              />
            ) : null}
            <Icon name={activity.icon} size={17} className="shrink-0" />
            {activityBarExpanded ? <span className="truncate">{activity.label}</span> : null}
          </button>
        )
        return activityBarExpanded ? (
          <div key={activity.id}>{button}</div>
        ) : (
          <Tip key={activity.id} label={activity.label} side="right">
            {button}
          </Tip>
        )
      })}

      <div className="flex-1" />

      <WorkspaceSelector
        workspaces={workspaces}
        activeWorkspace={activeWorkspace}
        onSelect={onSelectWorkspace}
        expanded={activityBarExpanded}
      />
      <RailPreferencesAndAvatar
        orgSlug={orgSlug}
        session={session}
        canAccessOrgSettings={canAccessOrgSettings}
        expanded={activityBarExpanded}
      />

      <Tip label={activityBarExpanded ? 'Collapse sidebar' : 'Expand sidebar'} side="right">
        <button
          type="button"
          onClick={() => setActivityBarExpanded(!activityBarExpanded)}
          aria-label="Toggle activity bar"
          className={cn(
            'flex items-center justify-center rounded-[calc(var(--radius-sm)+2px)] text-muted-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground',
            activityBarExpanded ? 'h-8 w-full' : 'size-8',
          )}
        >
          <Icon name="sidebar-left" size={17} className="shrink-0" />
        </button>
      </Tip>
    </nav>
  )
}

function IdeBrand({ expanded }: { expanded: boolean }) {
  const brand = useBrand()
  if (expanded) {
    return (
      <div className="mb-1 border-b border-border pb-2">
        <Link
          to="/"
          className="flex items-center gap-2.5 rounded-[calc(var(--radius-sm)+2px)] p-2 py-2.5 text-foreground transition-colors hover:bg-sidebar-accent/60"
          aria-label={`${brand.productName} home`}
        >
          <brand.LogoLockup size={16} className="shrink-0" />
        </Link>
      </div>
    )
  }
  return (
    <div className="mb-1 border-b border-border pb-2">
      <Tip label="Back to dashboard" side="right">
        <Link
          to="/"
          className="flex size-8 items-center justify-center rounded-[calc(var(--radius-sm)+2px)] text-foreground transition-colors hover:bg-sidebar-accent/60"
          aria-label={`${brand.productName} home`}
        >
          <brand.LogoMark size={18} className="shrink-0" />
        </Link>
      </Tip>
    </div>
  )
}

function RailPreferencesAndAvatar({
  orgSlug,
  session,
  canAccessOrgSettings,
  expanded,
}: {
  orgSlug: string
  session: SessionResponse | undefined
  canAccessOrgSettings: boolean
  expanded: boolean
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { preferences, setPreferences } = useAppShellPreferences()

  const logout = useMutation({
    mutationFn: async () => api.post<void>('/api/v1/auth/logout'),
    onSettled: async () => {
      clearAccessToken()
      clearAuthScopedQueryCache(queryClient)
      await navigate({ to: '/login', replace: true })
    },
  })

  if (!session) return null
  const menuItems = buildUserMenuItems({ session, orgSlug, canAccessOrgSettings })

  const avatarTrigger = (
    <DropdownMenuTrigger
      aria-label={session.account.name}
      className={cn(
        'flex cursor-pointer items-center rounded-[calc(var(--radius-sm)+2px)] text-xs transition-colors',
        'hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        expanded ? 'h-12 w-full gap-2 p-2' : 'size-8 justify-center',
      )}
    >
      <UserAvatar value={session.account.name} fallback="U" size={28} />
      {expanded ? (
        <div className="grid min-w-0 flex-1 text-left leading-tight">
          <span className="truncate font-medium">{session.account.name}</span>
          <span className="truncate text-muted-foreground">{session.account.email}</span>
        </div>
      ) : null}
    </DropdownMenuTrigger>
  )

  return (
    <>
      <AppShellPreferencesPopover
        preferences={preferences}
        setPreferences={setPreferences}
        isAdmin={session.is_instance_admin}
        buttonLabel={expanded ? 'UI Lab' : ''}
        buttonClassName={
          expanded ? 'h-8 w-full justify-start gap-2 p-2 text-xs' : 'size-8 justify-center px-0'
        }
      />

      <DropdownMenu>
        {expanded ? (
          avatarTrigger
        ) : (
          <Tip label={session.account.name} side="right">
            {avatarTrigger}
          </Tip>
        )}
        <DropdownMenuContent align="start" side="right" className="w-64 min-w-64">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="px-2 py-2">
              <div className="flex items-center gap-2 normal-case tracking-normal">
                <UserAvatar value={session.account.name} fallback="U" size={28} />
                <div className="flex min-w-0 flex-col gap-0.5">
                  <span className="truncate text-sm font-medium text-foreground">
                    {session.account.name}
                  </span>
                  <span className="truncate text-xs font-normal text-muted-foreground">
                    {session.account.email}
                  </span>
                </div>
              </div>
            </DropdownMenuLabel>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            {menuItems.map((item) => (
              <DropdownMenuItem
                key={item.id}
                render={<Link to={item.to as never} params={item.params as never} />}
              >
                <Icon name={item.icon} size={20} />
                {item.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            disabled={logout.isPending}
            onClick={() => {
              logout.mutate()
            }}
          >
            <Icon name="logout-03" size={20} />
            {logout.isPending ? 'Signing out...' : 'Sign out'}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  )
}
