import { useMemo } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Briefcase, Building2, LogOut, PanelsLeftRight, Settings, User, Wrench } from 'lucide-react'
import { useSession } from '#/hooks/use-session'
import { api } from '#/lib/api/client'
import { clearAccessToken, getAccessToken } from '#/lib/auth/access-token'
import { queryKeys } from '#/lib/api/query'
import { useLayoutWidth } from '#/components/layout-width-provider'
import { Button } from '#/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import ThemeToggle from './ThemeToggle'

export default function Header() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const { isExpanded, toggleMode } = useLayoutWidth()

  const logout = useMutation({
    mutationFn: async () => api.post<void>('/api/v1/auth/logout'),
    onSettled: async () => {
      clearAccessToken()
      await queryClient.invalidateQueries({ queryKey: queryKeys.session() })
      await navigate({ to: '/login', replace: true })
    },
  })

  const initials = useMemo(() => {
    const name = session.data?.account.name?.trim()
    if (!name) return 'U'
    const parts = name.split(/\s+/).filter(Boolean)
    return parts.slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('') || 'U'
  }, [session.data?.account.name])

  return (
    <header className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <nav className={isExpanded ? 'flex h-16 items-center justify-between px-4 sm:px-6' : 'container mx-auto flex h-16 items-center justify-between px-4'}>
        <Link to="/" className="flex items-center gap-2 text-lg font-semibold">
          <span className="h-2 w-2 rounded-full bg-primary" />
          SQLWarden
        </Link>

        <div className="flex items-center gap-3">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="cursor-pointer"
            onClick={toggleMode}
            aria-label={isExpanded ? 'Contract layout width' : 'Expand layout width'}
            title={isExpanded ? 'Contract layout width' : 'Expand layout width'}
          >
            <PanelsLeftRight />
          </Button>
          <ThemeToggle />

          <DropdownMenu>
            <DropdownMenuTrigger className="inline-flex h-9 cursor-pointer items-center gap-3 rounded-md px-1 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50">
              <span className="inline-flex size-8 items-center justify-center rounded-full bg-muted text-xs font-medium text-foreground">
                {initials}
              </span>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64 min-w-64">
              <DropdownMenuGroup>
                <DropdownMenuLabel className="px-2 py-2">
                  <div className="space-y-0.5">
                    <div className="text-sm font-medium text-foreground">
                      {session.data?.account.name ?? 'Account'}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {session.data?.account.email ?? 'Not signed in'}
                    </div>
                  </div>
                </DropdownMenuLabel>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuItem render={<Link to="/account" />}>
                <User className="size-4" />
                Profile
              </DropdownMenuItem>
              {session.data?.personal_spaces_enabled ? (
                <DropdownMenuItem disabled>
                  <Briefcase className="size-4" />
                  Workspaces
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuItem render={<Link to="/organizations" />}>
                <Building2 className="size-4" />
                Organizations
              </DropdownMenuItem>
              {session.data?.is_instance_admin ? (
                <DropdownMenuItem render={<Link to="/administration/overview" />}>
                  <Wrench className="size-4" />
                  Administration
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuItem disabled>
                <Settings className="size-4" />
                Settings
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                disabled={logout.isPending}
                onClick={() => {
                  void logout.mutateAsync()
                }}
              >
                <LogOut className="size-4" />
                {logout.isPending ? 'Signing out…' : 'Sign out'}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </nav>
    </header>
  )
}
