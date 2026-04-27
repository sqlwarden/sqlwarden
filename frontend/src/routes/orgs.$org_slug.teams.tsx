import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Outlet, createFileRoute, useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Cancel01Icon, PlusSignIcon, Search01Icon, UserGroupIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgTeamsQueryOptions } from '#/lib/api/query'
import type { ListQuery, Team } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { Avatar, AvatarFallback } from '#/components/ui/avatar'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { RoutePending } from '#/components/RoutePending'
import { Skeleton } from '#/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/orgs/$org_slug/teams')({
  component: OrganizationTeamsRoute,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function OrganizationTeamsRoute() {
  const { org_slug: orgSlug } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const listPath = `/orgs/${orgSlug}/teams`

  if (trimTrailingSlash(pathname) !== listPath) {
    return <Outlet />
  }

  return <OrganizationTeamsPage orgSlug={orgSlug} />
}

function OrganizationTeamsPage({ orgSlug }: { orgSlug: string }) {
  const queryClient = useQueryClient()
  const [searchText, setSearchText] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const [teamName, setTeamName] = useState('')
  const [teamSlug, setTeamSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; slug?: string }>({})
  const [query, setQuery] = useState<ListQuery>({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQuery((current) => {
        const nextSearch = searchText.trim()
        if ((current.q ?? '') === nextSearch) {
          return current
        }

        return {
          ...current,
          page: 1,
          q: nextSearch,
        }
      })
    }, 300)

    return () => window.clearTimeout(timer)
  }, [searchText])

  useEffect(() => {
    if (slugTouched) {
      return
    }

    setTeamSlug(slugify(teamName))
  }, [teamName, slugTouched])

  const teams = useQuery(orgTeamsQueryOptions(orgSlug, query))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canCreateTeam = hasPermission(effectivePermissions.data?.permissions, permission.orgWrite)
  const data = teams.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!teams.error) {
      return
    }

    toast.error(teams.error instanceof Error ? teams.error.message : 'Failed to load teams')
  }, [teams.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }

    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load team permissions')
  }, [effectivePermissions.error])

  const createTeam = useMutation({
    mutationFn: async () =>
      api.post<Team>(`/api/v1/orgs/${orgSlug}/teams`, {
        name: teamName.trim(),
        slug: teamSlug.trim(),
      }),
    onSuccess: async () => {
      setIsCreating(false)
      resetCreateTeam()
      toast.success('Team created')
      await queryClient.invalidateQueries({ queryKey: ['org-teams', orgSlug] })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          name: error.fieldErrors?.name,
          slug: error.fieldErrors?.slug,
        })
        if (error.fieldErrors?.name || error.fieldErrors?.slug) {
          return
        }
      }

      toast.error(error instanceof Error ? error.message : 'Failed to create team')
    },
  })

  function clearSearch() {
    setSearchText('')
  }

  function resetCreateTeam() {
    setTeamName('')
    setTeamSlug('')
    setSlugTouched(false)
    setFieldErrors({})
  }

  function submitCreateTeam(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const errors: { name?: string; slug?: string } = {}
    if (!teamName.trim()) {
      errors.name = 'Name is required'
    }
    if (!teamSlug.trim()) {
      errors.slug = 'Slug is required'
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      return
    }

    setFieldErrors({})
    void createTeam.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">Teams</h1>
            <p className="text-sm text-muted-foreground">Groups used for organization access control.</p>
          </div>

          {canCreateTeam ? (
            <Dialog
              open={isCreating}
              onOpenChange={(open) => {
                setIsCreating(open)
                if (!open) {
                  resetCreateTeam()
                }
              }}
            >
              <DialogTrigger render={<Button />}>
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
                Create
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create team</DialogTitle>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-4" onSubmit={submitCreateTeam}>
                  <div className="flex flex-col gap-2">
                    <Input
                      value={teamName}
                      onChange={(event) => {
                        setTeamName(event.target.value)
                        setFieldErrors((current) => ({ ...current, name: undefined }))
                      }}
                      placeholder="Team name"
                      aria-invalid={fieldErrors.name ? true : undefined}
                      disabled={createTeam.isPending}
                    />
                    {fieldErrors.name ? <p className="text-sm text-destructive">{fieldErrors.name}</p> : null}
                  </div>

                  <div className="flex flex-col gap-2">
                    <Input
                      value={teamSlug}
                      onChange={(event) => {
                        setSlugTouched(true)
                        setTeamSlug(slugify(event.target.value))
                        setFieldErrors((current) => ({ ...current, slug: undefined }))
                      }}
                      placeholder="team-slug"
                      aria-invalid={fieldErrors.slug ? true : undefined}
                      disabled={createTeam.isPending}
                    />
                    {fieldErrors.slug ? <p className="text-sm text-destructive">{fieldErrors.slug}</p> : null}
                  </div>

                  <DialogFooter>
                    <DialogClose render={<Button type="button" variant="ghost" disabled={createTeam.isPending} />}>
                      Cancel
                    </DialogClose>
                    <Button type="submit" disabled={createTeam.isPending}>
                      {createTeam.isPending ? 'Creating...' : 'Create'}
                    </Button>
                  </DialogFooter>
                </form>
              </DialogContent>
            </Dialog>
          ) : null}
        </div>

        <div className="relative max-w-md">
          <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(event) => setSearchText(event.target.value)}
            placeholder="Search teams"
            className="pe-9 ps-9"
          />
          {searchText ? (
            <button
              type="button"
              aria-label="Clear search"
              className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
              onClick={clearSearch}
            >
              <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-4" />
            </button>
          ) : null}
        </div>
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Team</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {teams.isLoading ? <TeamsTableSkeleton /> : null}
              {teams.isError ? <TeamsMessageRow message="Failed to load teams." /> : null}
              {!teams.isLoading && !teams.isError && items.length === 0 ? (
                <TeamsMessageRow message={query.q ? 'No teams matched your search.' : 'No teams found.'} />
              ) : null}
              {!teams.isLoading && !teams.isError
                ? items.map((team) => <TeamRow key={team.id} team={team} />)
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!teams.isLoading && !teams.isError && items.length > 0 ? (
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            {total === 0 ? '0 teams' : `${(page - 1) * pageSize + 1}-${Math.min(page * pageSize, total)} of ${total} teams`}
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={() => setQuery((current) => ({ ...current, page: Math.max(1, Number(current.page ?? 1) - 1) }))}
              disabled={page <= 1 || teams.isFetching}
            >
              Previous
            </Button>
            <div className="min-w-20 text-center text-sm text-muted-foreground">
              Page {page} of {pageCount}
            </div>
            <Button
              variant="outline"
              onClick={() => setQuery((current) => ({ ...current, page: Number(current.page ?? 1) + 1 }))}
              disabled={page >= pageCount || teams.isFetching}
            >
              Next
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function TeamRow({ team }: { team: Team }) {
  const { org_slug: orgSlug } = Route.useParams()
  const router = useRouter()
  const navigate = useNavigate()

  function preloadTeam() {
    void router.preloadRoute({
      to: '/orgs/$org_slug/teams/$team_slug',
      params: { org_slug: orgSlug, team_slug: team.slug },
    })
  }

  function openTeam() {
    void navigate({
      to: '/orgs/$org_slug/teams/$team_slug',
      params: { org_slug: orgSlug, team_slug: team.slug },
    })
  }

  return (
    <TableRow
      className="cursor-pointer"
      tabIndex={0}
      role="link"
      onFocus={preloadTeam}
      onMouseEnter={preloadTeam}
      onClick={openTeam}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          openTeam()
        }
      }}
    >
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <Avatar>
            <AvatarFallback>{teamInitials(team.name)}</AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{team.name}</div>
            <div className="truncate text-muted-foreground">@{team.slug}</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(team.created_at)}</TableCell>
    </TableRow>
  )
}

function TeamsTableSkeleton() {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-full" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}

function TeamsMessageRow({ message }: { message: string }) {
  return (
    <TableRow>
      <TableCell colSpan={2}>
        <div className="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
          <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} className="size-10 text-muted-foreground" />
          <p className="font-medium text-foreground">{message}</p>
        </div>
      </TableCell>
    </TableRow>
  )
}

function teamInitials(value: string) {
  const parts = value.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return 'T'
  }

  return parts
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return dateFormatter.format(date)
}

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
