import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon, UserGroupIcon, UserMultipleIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { orgEffectivePermissionsQueryOptions, orgMemberQueryOptions, orgMemberTeamsQueryOptions, orgTeamsQueryOptions } from '#/lib/api/query'
import type { Team } from '#/lib/api/types'
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
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '#/components/ui/breadcrumb'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { InitialsAvatar } from '#/components/InitialsAvatar'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Skeleton } from '#/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/orgs/$org_slug/users/$account_id')({
  component: OrganizationUserContextPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function OrganizationUserContextPage() {
  const { org_slug: orgSlug, account_id: accountId } = Route.useParams()
  const queryClient = useQueryClient()
  const [isAddingTeam, setIsAddingTeam] = useState(false)
  const { searchText, setSearchText, debouncedQuery: teamSearch, clearSearch } = useDebouncedQueryText()
  const { query: teamsQuery, toggleSort: toggleTeamsSort } = useListPageState({
    page: 1,
    page_size: 25,
    sort: 'name',
    order: 'asc',
  })
  const member = useQuery(orgMemberQueryOptions(orgSlug, accountId))
  const teams = useQuery(orgMemberTeamsQueryOptions(orgSlug, accountId, teamsQuery))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const orgTeams = useQuery({
    ...orgTeamsQueryOptions(orgSlug, {
      page: 1,
      page_size: 8,
      sort: 'name',
      order: 'asc',
      q: teamSearch,
    }),
    enabled: isAddingTeam,
  })
  const teamItems = teams.data?.items ?? []
  const existingTeamSlugs = new Set(teamItems.map((team) => team.slug))
  const displayName = member.data?.name || member.data?.email || `User #${accountId}`
  const canManageTeams = hasPermission(effectivePermissions.data?.permissions, permission.orgWrite)

  useEffect(() => {
    if (!member.error) {
      return
    }
    toast.error(member.error instanceof Error ? member.error.message : 'Failed to load user')
  }, [member.error])

  useEffect(() => {
    if (!teams.error) {
      return
    }
    toast.error(teams.error instanceof Error ? teams.error.message : 'Failed to load user teams')
  }, [teams.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load user permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!orgTeams.error) {
      return
    }
    toast.error(orgTeams.error instanceof Error ? orgTeams.error.message : 'Failed to load teams')
  }, [orgTeams.error])

  const addTeam = useMutation({
    mutationFn: async (teamSlug: string) =>
      api.post<void>(`/api/v1/orgs/${orgSlug}/teams/${teamSlug}/members`, {
        account_id: Number(accountId),
      }),
    onSuccess: async (_, teamSlug) => {
      toast.success('Done')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['org-member-teams', orgSlug, accountId] }),
        queryClient.invalidateQueries({ queryKey: ['org-team-members', orgSlug, teamSlug] }),
      ])
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to add team')
    },
  })

  const removeTeam = useMutation({
    mutationFn: async (teamSlug: string) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/teams/${teamSlug}/members/${accountId}`),
    onSuccess: async (_, teamSlug) => {
      toast.success('Done')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['org-member-teams', orgSlug, accountId] }),
        queryClient.invalidateQueries({ queryKey: ['org-team-members', orgSlug, teamSlug] }),
      ])
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to remove team')
    },
  })

  function resetAddTeam() {
    clearSearch()
  }

  return (
    <div className="flex flex-col gap-8">
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink render={<Link to="/orgs/$org_slug/users" params={{ org_slug: orgSlug }} />}>
              Users
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{displayName}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <Card className="overflow-hidden">
        {member.isLoading ? (
          <div className="flex items-center gap-4 border-b border-border bg-muted/30 px-6 py-5">
            <Skeleton className="size-12 shrink-0 rounded-full" />
            <div className="flex flex-col gap-2">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-56" />
            </div>
          </div>
        ) : null}

        {member.isError ? (
          <CardContent>
            <ContextMessage icon={UserMultipleIcon} message="Failed to load user." />
          </CardContent>
        ) : null}

        {member.data ? (
          <>
            <div className="flex items-center gap-4 border-b border-border bg-muted/30 px-6 py-5">
              <InitialsAvatar value={member.data.name || member.data.email} size="lg" />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="text-xl font-semibold leading-tight tracking-tight">{displayName}</h1>
                  <Badge variant={roleBadgeVariant(member.data.role)}>{roleLabel(member.data.role)}</Badge>
                </div>
                <p className="mt-0.5 truncate text-sm text-muted-foreground">{member.data.email}</p>
              </div>
            </div>
            <div className="grid gap-5 px-6 py-5 sm:grid-cols-2">
              <InfoBlock label="Account ID" value={String(member.data.account_id)} />
              <InfoBlock label="Joined" value={formatDate(member.data.joined_at)} />
            </div>
          </>
        ) : null}
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between border-b border-border">
          <CardTitle>Teams</CardTitle>
          {canManageTeams ? (
            <Dialog
              open={isAddingTeam}
              onOpenChange={(open) => {
                setIsAddingTeam(open)
                if (!open) {
                  resetAddTeam()
                }
              }}
            >
              <DialogTrigger render={<Button />}>
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
                Add Team
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Add Team</DialogTitle>
                </DialogHeader>
                <div className="mt-6 flex flex-col gap-4">
                  <SearchInput
                    value={searchText}
                    onValueChange={setSearchText}
                    onClear={clearSearch}
                    placeholder="Search teams"
                    className="max-w-none"
                  />

                  <div className="min-h-64">
                    <Table>
                      <TableBody>
                        {orgTeams.isLoading ? <TeamsPickerSkeleton /> : null}
                        {orgTeams.isError ? <MessageRow colSpan={2} icon={UserGroupIcon} message="Failed to load teams." /> : null}
                        {!orgTeams.isLoading && !orgTeams.isError && (orgTeams.data?.items ?? []).length === 0 ? (
                          <MessageRow colSpan={2} icon={UserGroupIcon} message="No teams found." />
                        ) : null}
                        {!orgTeams.isLoading && !orgTeams.isError
                          ? (orgTeams.data?.items ?? []).map((team) => (
                              <TeamPickerRow
                                key={team.id}
                                team={team}
                                isExistingTeam={existingTeamSlugs.has(team.slug)}
                                isPending={addTeam.isPending}
                                onAdd={(teamSlug) => addTeam.mutate(teamSlug)}
                              />
                            ))
                          : null}
                      </TableBody>
                    </Table>
                  </div>
                </div>
                <DialogFooter>
                  <DialogClose render={<Button type="button" variant="ghost" />}>
                    Close
                  </DialogClose>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          ) : null}
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Team" sort="name" currentSort={teamsQuery.sort} currentOrder={teamsQuery.order} onSortChange={toggleTeamsSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={teamsQuery.sort} currentOrder={teamsQuery.order} onSortChange={toggleTeamsSort} />
                </TableHead>
                {canManageTeams ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {teams.isLoading ? <TeamsTableSkeleton /> : null}
              {teams.isError ? <MessageRow colSpan={canManageTeams ? 3 : 2} icon={UserGroupIcon} message="Failed to load teams." /> : null}
              {!teams.isLoading && !teams.isError && teamItems.length === 0 ? (
                <MessageRow colSpan={canManageTeams ? 3 : 2} icon={UserGroupIcon} message="This user does not belong to any teams." />
              ) : null}
              {!teams.isLoading && !teams.isError
                ? teamItems.map((team) => (
                    <TeamRow
                      key={team.id}
                      orgSlug={orgSlug}
                      team={team}
                      canManageTeams={canManageTeams}
                      isRemoving={removeTeam.isPending}
                      onRemove={(teamSlug) => removeTeam.mutate(teamSlug)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
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
  onAdd: (teamSlug: string) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <InitialsAvatar value={team.name} fallback="T" />
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{team.name}</div>
            <div className="truncate text-muted-foreground">@{team.slug}</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-end">
        <Button
          variant={isExistingTeam ? 'outline' : 'default'}
          disabled={isExistingTeam || isPending}
          onClick={() => onAdd(team.slug)}
        >
          {isExistingTeam ? 'Added' : 'Add'}
        </Button>
      </TableCell>
    </TableRow>
  )
}

function TeamsPickerSkeleton() {
  return (
    <>
      {Array.from({ length: 4 }).map((_, index) => (
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
          <TableCell className="text-end">
            <Skeleton className="ms-auto h-7 w-14" />
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}

function TeamRow({
  orgSlug,
  team,
  canManageTeams,
  isRemoving,
  onRemove,
}: {
  orgSlug: string
  team: Team
  canManageTeams: boolean
  isRemoving: boolean
  onRemove: (teamSlug: string) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <Link
          to="/orgs/$org_slug/teams/$team_slug"
          params={{ org_slug: orgSlug, team_slug: team.slug }}
          className="flex min-w-0 items-center gap-3"
        >
          <InitialsAvatar value={team.name} fallback="T" />
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{team.name}</div>
            <div className="truncate text-muted-foreground">@{team.slug}</div>
          </div>
        </Link>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(team.created_at)}</TableCell>
      {canManageTeams ? (
        <TableCell className="text-end">
          <AlertDialog>
            <AlertDialogTrigger render={<Button variant="destructive" disabled={isRemoving} />}>
              Remove
            </AlertDialogTrigger>
            <AlertDialogContent size="sm">
              <AlertDialogHeader>
                <AlertDialogTitle>Remove team from user?</AlertDialogTitle>
                <AlertDialogDescription>
                  This will remove the user from {team.name}. Access granted through this team will no longer apply.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel variant="ghost" disabled={isRemoving}>
                  Cancel
                </AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  disabled={isRemoving}
                  onClick={() => onRemove(team.slug)}
                >
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

function TeamsTableSkeleton() {
  return (
    <>
      {Array.from({ length: 3 }).map((_, index) => (
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

function InfoBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 border-l-2 border-border pl-3">
      <span className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">{label}</span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
  )
}

function ContextMessage({ icon, message }: { icon: Parameters<typeof HugeiconsIcon>[0]['icon']; message: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-center">
      <HugeiconsIcon icon={icon} strokeWidth={2} className="size-10 text-muted-foreground" />
      <p className="font-medium text-foreground">{message}</p>
    </div>
  )
}

function MessageRow({ colSpan, icon, message }: { colSpan: number; icon: Parameters<typeof HugeiconsIcon>[0]['icon']; message: string }) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan}>
        <ContextMessage icon={icon} message={message} />
      </TableCell>
    </TableRow>
  )
}

function roleBadgeVariant(role: string): 'default' | 'secondary' | 'outline' {
  if (role === 'owner') {
    return 'default'
  }
  if (role === 'admin') {
    return 'secondary'
  }
  return 'outline'
}

function roleLabel(role: string) {
  if (!role) {
    return 'Member'
  }
  return role.charAt(0).toUpperCase() + role.slice(1)
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return dateFormatter.format(date)
}
