import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { AppShellPreferencesPopover, useAppShellPreferences } from '#/components/app-shell'
import { InitialsAvatar } from '#/components/InitialsAvatar'
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
import type { SessionResponse } from '#/lib/api/types'

export function IdeTopBarControls({
  orgSlug,
  session,
  canAccessOrgSettings,
}: {
  orgSlug: string
  session: SessionResponse
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

  return (
    <div className="flex shrink-0 items-center gap-1 border-l border-border px-2">
      <AppShellPreferencesPopover
        preferences={preferences}
        setPreferences={setPreferences}
        buttonLabel=""
        buttonClassName="size-7 justify-center px-0"
        contentSide="bottom"
        contentAlign="end"
      />

      <DropdownMenu>
        <DropdownMenuTrigger className="inline-flex cursor-pointer items-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          <InitialsAvatar value={session.account.name} fallback="U" className="size-7 rounded-full" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64 min-w-64">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="px-2 py-2">
              <div className="flex flex-col gap-0.5 normal-case tracking-normal">
                <span className="truncate text-sm font-medium text-foreground">{session.account.name}</span>
                <span className="truncate text-xs font-normal text-muted-foreground">{session.account.email}</span>
              </div>
            </DropdownMenuLabel>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            {canAccessOrgSettings ? (
              <DropdownMenuItem render={<Link to="/orgs/$org_slug" params={{ org_slug: orgSlug }} />}>
                <Icon name="settings-02" size={20} />
                Organization Settings
              </DropdownMenuItem>
            ) : null}
            <DropdownMenuItem render={<Link to="/" />}>
              <Icon name="building-04" size={20} />
              Switch Organization
            </DropdownMenuItem>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            disabled={logout.isPending}
            onClick={() => {
              void logout.mutateAsync()
            }}
          >
            <Icon name="logout-03" size={20} />
            {logout.isPending ? 'Signing out...' : 'Sign out'}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
