import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Outlet, createFileRoute, useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon, UserGroupIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgTeamsQueryOptions } from '#/lib/api/query'
import type { Team } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { InitialsAvatar } from '#/components/InitialsAvatar'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'
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
  const [isCreating, setIsCreating] = useState(false)
  const [teamName, setTeamName] = useState('')
  const [teamSlug, setTeamSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; slug?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

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
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">Teams</h1>
            <p className="text-sm text-muted-foreground">
              {!teams.isLoading && total > 0
                ? `${total} team${total !== 1 ? 's' : ''} in this organization`
                : 'Groups used for organization access control.'}
            </p>
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

        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search teams"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Team" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {teams.isLoading ? <TeamsTableSkeleton /> : null}
              {teams.isError ? <TableEmptyState colSpan={2} icon={UserGroupIcon} message="Failed to load teams." /> : null}
              {!teams.isLoading && !teams.isError && items.length === 0 ? (
                <TableEmptyState colSpan={2} icon={UserGroupIcon} message={query.q ? 'No teams matched your search.' : 'No teams found.'} />
              ) : null}
              {!teams.isLoading && !teams.isError
                ? items.map((team) => <TeamRow key={team.id} team={team} />)
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!teams.isLoading && !teams.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="teams"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={teams.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
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
          <InitialsAvatar value={team.name} />
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
