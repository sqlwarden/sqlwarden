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
      <IdeBrand />

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

      <div className="flex-1" />

      <WorkspaceSelector
        workspaces={workspaces}
        activeWorkspace={activeWorkspace}
        onSelect={onSelectWorkspace}
      />
      <RailPreferencesAndAvatar
        orgSlug={orgSlug}
        session={session}
        canAccessOrgSettings={canAccessOrgSettings}
      />
    </nav>
  )
}

function IdeBrand() {
  const brand = useBrand()
  return (
    <Tip label="Back to dashboard" side="right">
      <Link
        to="/"
        className="flex size-8 items-center justify-center rounded-lg text-foreground transition-colors hover:bg-sidebar-accent/60"
        aria-label={`${brand.productName} home`}
      >
        <brand.LogoMark size={18} className="shrink-0" />
      </Link>
    </Tip>
  )
}

function RailPreferencesAndAvatar({
  orgSlug,
  session,
  canAccessOrgSettings,
}: {
  orgSlug: string
  session: SessionResponse | undefined
  canAccessOrgSettings: boolean
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

  return (
    <>
      <AppShellPreferencesPopover
        preferences={preferences}
        setPreferences={setPreferences}
        isAdmin={session.is_instance_admin}
        buttonLabel=""
        buttonClassName="size-8 justify-center px-0"
      />

      <DropdownMenu>
        <Tip label={session.account.name} side="right">
          <DropdownMenuTrigger
            aria-label={session.account.name}
            className="inline-flex cursor-pointer items-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <UserAvatar value={session.account.name} fallback="U" size={28} />
          </DropdownMenuTrigger>
        </Tip>
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
