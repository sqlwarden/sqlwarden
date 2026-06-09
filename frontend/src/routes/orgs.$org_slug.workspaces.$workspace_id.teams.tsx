import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate, useRouter } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import {
  orgEffectivePermissionsQueryOptions,
  orgTeamsQueryOptions,
  orgWorkspaceTeamsQueryOptions,
} from '#/lib/api/query'
import type { Team, WorkspaceTeam } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '#/components/ui/alert-dialog'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { SectionTabNav } from '#/components/SectionTabNav'
import { entityColor } from '#/lib/entity-colors'
import { cn } from '#/lib/utils'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Skeleton } from '#/components/ui/skeleton'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/teams')({
  component: WorkspaceTeamsPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function WorkspaceTeamsPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const queryClient = useQueryClient()
  const [isAddingTeam, setIsAddingTeam] = useState(false)
  const { searchText: pickerSearchText, setSearchText: setPickerSearchText, debouncedQuery: pickerSearch, clearSearch: clearPickerSearch } = useDebouncedQueryText()
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const canReadTeams = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canModifyTeams = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)
  const teams = useQuery({
    ...orgWorkspaceTeamsQueryOptions(orgSlug, workspaceId, query),
    enabled: canReadTeams,
  })
  const orgTeams = useQuery({
    ...orgTeamsQueryOptions(orgSlug, {
      page: 1,
      page_size: 10,
      sort: 'name',
      order: 'asc',
      q: pickerSearch,
    }),
    enabled: isAddingTeam && canModifyTeams,
  })

  const items = teams.data?.items ?? []
  const existingTeamIds = new Set(items.map((team) => team.team_id))
  const page = teams.data?.page ?? Number(query.page ?? 1)
  const pageSize = teams.data?.page_size ?? Number(query.page_size ?? 10)
  const total = teams.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1
  const tableColumnCount = canModifyTeams ? 4 : 3

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load team permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!canReadTeams || !teams.error) return
    toast.error(teams.error instanceof Error ? teams.error.message : 'Failed to load workspace teams')
  }, [canReadTeams, teams.error])

  useEffect(() => {
    if (!isAddingTeam || !canModifyTeams || !orgTeams.error) return
    toast.error(orgTeams.error instanceof Error ? orgTeams.error.message : 'Failed to load organization teams')
  }, [canModifyTeams, isAddingTeam, orgTeams.error])

  const addTeam = useMutation({
    mutationFn: async (teamId: number) =>
      api.post<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/teams`, { team_id: teamId }),
    onSuccess: async () => {
      toast.success('Team added')
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-teams', orgSlug, workspaceId] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to add team')
    },
  })

  const removeTeam = useMutation({
    mutationFn: async (teamId: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/teams/${teamId}`),
    onSuccess: async () => {
      toast.success('Team removed')
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-teams', orgSlug, workspaceId] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to remove team')
    },
  })

  return (
    <div className="flex flex-col">
      <SectionTabNav
        tabs={[
          { label: 'Users', to: '/orgs/$org_slug/workspaces/$workspace_id/users', params: { org_slug: orgSlug, workspace_id: workspaceId }, isActive: false },
          { label: 'Teams', to: '/orgs/$org_slug/workspaces/$workspace_id/teams', params: { org_slug: orgSlug, workspace_id: workspaceId }, isActive: true },
        ]}
      />

      <div className="flex flex-col gap-6 pt-6">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            {!teams.isLoading && total > 0
              ? `${total} team${total !== 1 ? 's' : ''} in this workspace`
              : 'Teams explicitly added to this workspace.'}
          </p>

          {canModifyTeams ? (
            <Dialog
              open={isAddingTeam}
              onOpenChange={(open) => {
                setIsAddingTeam(open)
                if (!open) clearPickerSearch()
              }}
            >
              <DialogTrigger render={<Button />}>
                <Icon name="plus-sign" size={20} data-icon="inline-start" />
                Add Team
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Add Team</DialogTitle>
                </DialogHeader>
                <div className="mt-6 flex flex-col gap-4">
                  <SearchInput
                    value={pickerSearchText}
                    onValueChange={setPickerSearchText}
                    onClear={clearPickerSearch}
                    placeholder="Search organization teams"
                    className="max-w-none"
                  />
                  <div className="min-h-64">
                    <Table>
                      <TableBody>
                        {orgTeams.isLoading ? <TeamPickerSkeleton /> : null}
                        {orgTeams.isError ? <MessageRow colSpan={2} icon="user-group" message="Failed to load teams." /> : null}
                        {!orgTeams.isLoading && !orgTeams.isError && (orgTeams.data?.items ?? []).length === 0 ? (
                          <MessageRow colSpan={2} icon="user-group" message="No teams found." />
                        ) : null}
                        {!orgTeams.isLoading && !orgTeams.isError
                          ? (orgTeams.data?.items ?? []).map((team) => (
                              <TeamPickerRow
                                key={team.id}
                                team={team}
                                isExistingTeam={existingTeamIds.has(team.id)}
                                isPending={addTeam.isPending}
                                onAdd={(teamId) => addTeam.mutate(teamId)}
                              />
                            ))
                          : null}
                      </TableBody>
                    </Table>
                  </div>
                </div>
                <DialogFooter>
                  <DialogClose render={<Button type="button" variant="ghost" />}>Close</DialogClose>
                </DialogFooter>
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
                  <TableColumnHeader label="Members" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Added" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                {canModifyTeams ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectivePermissions.isLoading || teams.isLoading ? <TeamsTableSkeleton canModifyTeams={canModifyTeams} /> : null}
              {teams.isError ? <TableEmptyState colSpan={tableColumnCount} icon="user-group" message="Failed to load teams." /> : null}
              {!effectivePermissions.isLoading && !canReadTeams ? (
                <TableEmptyState colSpan={tableColumnCount} icon="user-group" message="You do not have permission to view workspace teams." />
              ) : null}
              {!effectivePermissions.isLoading && canReadTeams && !teams.isLoading && !teams.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={tableColumnCount}
                  icon="user-group"
                  message={query.q ? 'No teams matched your search.' : 'No workspace teams found.'}
                />
              ) : null}
              {!effectivePermissions.isLoading && canReadTeams && !teams.isLoading && !teams.isError
                ? items.map((team) => (
                    <WorkspaceTeamRow
                      key={team.team_id}
                      orgSlug={orgSlug}
                      team={team}
                      canModifyTeams={canModifyTeams}
                      isRemoving={removeTeam.isPending}
                      onRemove={(teamId) => removeTeam.mutate(teamId)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {canReadTeams && !teams.isLoading && !teams.isError && items.length > 0 ? (
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
    </div>
  )
}

function TeamPickerRow({
  team,
  isExistingTeam,
  isPending,
  onAdd,
}: {
  team: Team
  isExistingTeam: boolean
  isPending: boolean
  onAdd: (teamId: number) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md', entityColor(team.name))}>
            <Icon name="user-group" size={20} className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{team.name}</div>
            <div className="truncate text-sm text-muted-foreground">@{team.slug}</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-end">
        <Button
          variant={isExistingTeam ? 'outline' : 'default'}
          disabled={isExistingTeam || isPending}
          onClick={() => onAdd(team.id)}
        >
          {isExistingTeam ? 'Added' : 'Add'}
        </Button>
      </TableCell>
    </TableRow>
  )
}

function WorkspaceTeamRow({
  orgSlug,
  team,
  canModifyTeams,
  isRemoving,
  onRemove,
}: {
  orgSlug: string
  team: WorkspaceTeam
  canModifyTeams: boolean
  isRemoving: boolean
  onRemove: (teamId: number) => void
}) {
  const router = useRouter()
  const navigate = useNavigate()

  function preload() {
    void router.preloadRoute({
      to: '/orgs/$org_slug/teams/$team_slug',
      params: { org_slug: orgSlug, team_slug: team.slug },
    })
  }

  function open() {
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
      onFocus={preload}
      onMouseEnter={preload}
      onClick={open}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          open()
        }
      }}
    >
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md', entityColor(team.name))}>
            <Icon name="user-group" size={20} className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{team.name}</div>
            <div className="truncate text-sm text-muted-foreground">@{team.slug}</div>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant="outline">{team.member_count}</Badge>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(team.created_at)}</TableCell>
      {canModifyTeams ? (
        <TableCell className="text-end" onClick={(e) => e.stopPropagation()}>
          <AlertDialog>
            <AlertDialogTrigger render={<Button variant="destructive" disabled={isRemoving} />}>
              Remove
            </AlertDialogTrigger>
            <AlertDialogContent size="sm">
              <AlertDialogHeader>
                <AlertDialogTitle>Remove team from workspace?</AlertDialogTitle>
                <AlertDialogDescription>
                  This removes {team.name} from the workspace. Members will lose permissions granted through this workspace team membership.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel variant="ghost" disabled={isRemoving}>Cancel</AlertDialogCancel>
                <AlertDialogAction variant="destructive" disabled={isRemoving} onClick={() => onRemove(team.team_id)}>
                  Remove
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function TeamPickerSkeleton() {
  return (
    <>
      {Array.from({ length: 4 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-md" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          </TableCell>
          <TableCell className="text-end">
            <Skeleton className="ms-auto h-7 w-14" />
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}

function TeamsTableSkeleton({ canModifyTeams }: { canModifyTeams: boolean }) {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-md" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-5 w-12 rounded-full" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          {canModifyTeams ? (
            <TableCell className="text-end">
              <Skeleton className="ms-auto h-8 w-20" />
            </TableCell>
          ) : null}
        </TableRow>
      ))}
    </>
  )
}

function MessageRow({ colSpan, icon, message }: { colSpan: number; icon: import('#/lib/icons').AppIcon; message: string }) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan}>
        <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-center">
          <Icon name={icon} size={20} className="size-10 text-muted-foreground" />
          <p className="font-medium text-foreground">{message}</p>
        </div>
      </TableCell>
    </TableRow>
  )
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Unknown'
  return dateFormatter.format(date)
}
