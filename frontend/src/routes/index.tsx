import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Navigate, createFileRoute, useNavigate } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  Briefcase01Icon,
  Building04Icon,
  Logout03Icon,
  Settings02Icon,
  ShieldUserIcon,
  UserGroupIcon,
  UserMultipleIcon,
} from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { api } from '#/lib/api/client'
import { accountOrganizationsQueryOptions } from '#/lib/api/query'
import type { AccountOrganization, Organization, SessionResponse } from '#/lib/api/types'
import { clearAccessToken, getAccessToken } from '#/lib/auth/access-token'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { Badge } from '#/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { EmptyState } from '#/components/EmptyState'
import { InitialsAvatar, getInitials } from '#/components/InitialsAvatar'
import { Skeleton } from '#/components/ui/skeleton'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/')({ component: LandingPage })

function LandingPage() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const shouldLoadOrganizations = Boolean(hasToken && session.data && (session.data.personal_spaces_enabled || session.data.organizations.length !== 1))
  const organizations = useQuery({
    ...accountOrganizationsQueryOptions({
      page: 1,
      page_size: 50,
      sort: 'name',
      order: 'asc',
      q: '',
    }),
    enabled: shouldLoadOrganizations,
  })

  useEffect(() => {
    if (!organizations.error) {
      return
    }

    toast.error(organizations.error instanceof Error ? organizations.error.message : 'Failed to load organizations')
  }, [organizations.error])

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return <LandingLoading />
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (!hasToken || !session.data) {
    return <Navigate to="/login" replace />
  }

  if (!session.data.personal_spaces_enabled && session.data.organizations.length === 1) {
    return (
      <Navigate
        to="/orgs/$org_slug/workspaces"
        params={{ org_slug: session.data.organizations[0].slug }}
        replace
      />
    )
  }

  const organizationItems = (organizations.data?.items ?? session.data.organizations) as Array<AccountOrganization | Organization>

  return (
    <main className="mx-auto flex min-h-svh w-full max-w-5xl flex-col gap-8 px-4 py-8 md:px-6">
      <div className="flex items-center justify-between gap-4">
        <Link to="/" className="flex items-center gap-2 text-sm font-semibold tracking-tight">
          <span className="size-2 rounded-full bg-primary" />
          SQLWarden
        </Link>
        <LandingUserMenu session={session.data} />
      </div>

      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">Choose where to continue</h1>
        <p className="text-sm text-muted-foreground">
          Select an organization or administration area.
        </p>
      </div>

      {organizations.isLoading ? <LandingCardsSkeleton /> : null}

      {!organizations.isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {session.data.personal_spaces_enabled ? <PersonalSpaceCard /> : null}
          {organizationItems.map((organization) => (
            <OrganizationChoiceCard key={organization.id} organization={organization} />
          ))}
          {session.data.is_instance_admin ? <AdministrationChoiceCard /> : null}
        </div>
      ) : null}

      {!organizations.isLoading && !organizations.isError && !session.data.personal_spaces_enabled && organizationItems.length === 0 && !session.data.is_instance_admin ? (
        <Card>
          <CardContent>
            <EmptyState
              icon={Building04Icon}
              message="No work areas found."
              description="You are not a member of any organizations yet."
            />
          </CardContent>
        </Card>
      ) : null}
    </main>
  )
}

function OrganizationChoiceCard({ organization }: { organization: AccountOrganization | Organization }) {
  const memberCount = 'member_count' in organization ? organization.member_count : undefined
  const teamCount = 'team_count' in organization ? organization.team_count : undefined
  const role = 'role' in organization ? organization.role : undefined

  return (
    <Link
      to="/orgs/$org_slug/workspaces"
      params={{ org_slug: organization.slug }}
      className="group block h-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex h-full flex-col border border-border bg-card text-card-foreground transition-all group-hover:border-foreground/20 group-hover:bg-muted/20 group-hover:shadow-sm">
        <div className="flex flex-1 flex-col gap-3 p-5">
          <div className="flex min-w-0 items-start gap-3">
            <div className={cn('flex size-10 shrink-0 items-center justify-center text-sm font-semibold', organizationColor(organization.name))}>
              {getInitials(organization.name, 'O')}
            </div>
            <div className="min-w-0 flex-1 pt-0.5">
              <div className="flex min-w-0 items-center gap-2">
                <p className="min-w-0 flex-1 truncate font-semibold leading-tight tracking-tight transition-colors group-hover:text-primary">
                  {organization.name}
                </p>
                {role ? (
                  <Badge variant="outline" className="h-4 max-w-24 shrink-0 truncate px-1.5 py-0 text-[10px] capitalize">
                    {role}
                  </Badge>
                ) : null}
              </div>
              <p className="mt-1.5 truncate text-xs text-muted-foreground">@{organization.slug}</p>
            </div>
          </div>
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-x-5 gap-y-1 border-t border-border/60 px-5 py-3 text-xs text-muted-foreground">
          <div className="flex min-w-0 items-center gap-1.5 [&_svg]:size-3.5">
            <HugeiconsIcon icon={UserMultipleIcon} strokeWidth={2} />
            <span className="truncate">{formatCount(memberCount, 'member')}</span>
          </div>
          <div className="flex min-w-0 items-center gap-1.5 [&_svg]:size-3.5">
            <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} />
            <span className="truncate">{formatCount(teamCount, 'team')}</span>
          </div>
        </div>
      </div>
    </Link>
  )
}

function PersonalSpaceCard() {
  return (
    <div className="flex h-full flex-col border border-border bg-card text-card-foreground">
      <div className="flex flex-1 flex-col gap-3 p-5">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
            <HugeiconsIcon icon={Briefcase01Icon} strokeWidth={2} />
          </div>
          <div className="min-w-0 flex-1">
            <CardTitle className="truncate text-base">Personal Workspace</CardTitle>
            <CardDescription>Your private SQLWarden space.</CardDescription>
          </div>
        </div>
      </div>
      <div className="border-t border-border/60 px-5 py-3 text-xs text-muted-foreground">
        Coming soon
      </div>
    </div>
  )
}

function AdministrationChoiceCard() {
  return (
    <Link
      to="/settings/administrators"
      className="group block h-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex h-full flex-col border border-border bg-card text-card-foreground transition-all group-hover:border-foreground/20 group-hover:bg-muted/20 group-hover:shadow-sm">
        <div className="flex flex-1 flex-col gap-3 p-5">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center bg-muted text-muted-foreground">
              <HugeiconsIcon icon={ShieldUserIcon} strokeWidth={2} />
            </div>
            <div className="min-w-0 flex-1 pt-0.5">
              <p className="truncate font-semibold leading-tight tracking-tight transition-colors group-hover:text-primary">
                Administration
              </p>
              <p className="mt-1.5 truncate text-xs text-muted-foreground">Instance settings and users.</p>
            </div>
          </div>
        </div>
        <div className="border-t border-border/60 px-5 py-3 text-xs text-muted-foreground">
          Open instance administration
        </div>
      </div>
    </Link>
  )
}

function LandingUserMenu({ session }: { session: SessionResponse }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const logout = useMutation({
    mutationFn: async () => api.post<void>('/api/v1/auth/logout'),
    onSettled: async () => {
      clearAccessToken()
      clearAuthScopedQueryCache(queryClient)
      await navigate({ to: '/login', replace: true })
    },
  })

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="inline-flex cursor-pointer items-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        <InitialsAvatar value={session.account.name} fallback="U" className="size-9 rounded-full" />
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
        <DropdownMenuItem render={<Link to="/settings" />}>
          <HugeiconsIcon icon={Settings02Icon} strokeWidth={2} />
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
          <HugeiconsIcon icon={Logout03Icon} strokeWidth={2} />
          {logout.isPending ? 'Signing out...' : 'Sign out'}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function LandingLoading() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="text-sm text-muted-foreground">Loading...</div>
    </main>
  )
}

function LandingCardsSkeleton() {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: 3 }).map((_, index) => (
        <div key={index} className="flex flex-col border border-border bg-card">
          <div className="flex flex-col gap-3 p-5">
            <div className="flex items-start gap-3">
              <Skeleton className="size-10 shrink-0" />
              <div className="flex flex-1 flex-col gap-2 pt-1">
                <div className="flex items-center gap-2">
                  <Skeleton className="h-4 w-28" />
                  <Skeleton className="h-4 w-12" />
                </div>
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          </div>
          <div className="flex items-center gap-5 border-t border-border/60 px-5 py-3">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-3 w-16" />
          </div>
        </div>
      ))}
    </div>
  )
}

const ORGANIZATION_COLORS = [
  'bg-orange-500/10 text-orange-600',
  'bg-blue-500/10 text-blue-600',
  'bg-emerald-500/10 text-emerald-600',
  'bg-violet-500/10 text-violet-600',
  'bg-rose-500/10 text-rose-600',
  'bg-amber-500/10 text-amber-600',
  'bg-cyan-500/10 text-cyan-600',
]

function organizationColor(name: string): string {
  const hash = name.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
  return ORGANIZATION_COLORS[hash % ORGANIZATION_COLORS.length]
}

function formatCount(value: number | undefined, label: string) {
  if (value === undefined) {
    return `— ${label}s`
  }

  return `${value} ${value === 1 ? label : `${label}s`}`
}
