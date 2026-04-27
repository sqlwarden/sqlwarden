import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Cancel01Icon, PlusSignIcon, Search01Icon, UserGroupIcon, UserMultipleIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { orgEffectivePermissionsQueryOptions, orgMemberQueryOptions, orgMemberTeamsQueryOptions, orgTeamsQueryOptions } from '#/lib/api/query'
import type { ListQuery, Team } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { Avatar, AvatarFallback } from '#/components/ui/avatar'
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
import { Input } from '#/components/ui/input'
import { RoutePending } from '#/components/RoutePending'
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
  const [searchText, setSearchText] = useState('')
  const [teamSearch, setTeamSearch] = useState('')
  const [teamsQuery] = useState<ListQuery>({
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
    const timer = window.setTimeout(() => {
      setTeamSearch(searchText.trim())
    }, 300)

    return () => window.clearTimeout(timer)
  }, [searchText])

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
    setSearchText('')
    setTeamSearch('')
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

      <Card>
        <CardContent>
          {member.isLoading ? (
            <div className="flex items-center gap-4">
              <Skeleton className="size-12 rounded-full" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-5 w-40" />
                <Skeleton className="h-4 w-56" />
              </div>
            </div>
          ) : null}

          {member.isError ? (
            <ContextMessage icon={UserMultipleIcon} message="Failed to load user." />
          ) : null}

          {member.data ? (
            <div className="flex flex-col gap-6">
              <div className="flex items-start gap-4">
                <Avatar size="lg">
                  <AvatarFallback>{userInitials(member.data.name || member.data.email)}</AvatarFallback>
                </Avatar>
                <div className="min-w-0 flex-1">
                  <h1 className="truncate text-2xl font-semibold tracking-tight">{displayName}</h1>
                  <p className="truncate text-sm text-muted-foreground">{member.data.email}</p>
                </div>
                <Badge variant={roleBadgeVariant(member.data.role)}>{roleLabel(member.data.role)}</Badge>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <InfoBlock label="Account ID" value={String(member.data.account_id)} />
                <InfoBlock label="Joined" value={formatDate(member.data.joined_at)} />
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
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
                  <div className="relative">
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
                        onClick={() => setSearchText('')}
                      >
                        <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-4" />
                      </button>
                    ) : null}
                  </div>

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
                <TableHead>Team</TableHead>
                <TableHead>Created</TableHead>
                {canManageTeams ? <TableHead className="text-end">Actions</TableHead> : null}
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
          <Avatar>
            <AvatarFallback>{initials(team.name, 'T')}</AvatarFallback>
          </Avatar>
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
          <Avatar>
            <AvatarFallback>{initials(team.name, 'T')}</AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{team.name}</div>
            <div className="truncate text-muted-foreground">@{team.slug}</div>
          </div>
        </Link>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(team.created_at)}</TableCell>
      {canManageTeams ? (
        <TableCell className="text-end">
          <Button
            variant="ghost"
            disabled={isRemoving}
            onClick={() => onRemove(team.slug)}
          >
            Remove
          </Button>
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
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground uppercase">{label}</span>
      <span className="text-sm text-foreground">{value}</span>
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

function userInitials(value: string) {
  return initials(value, 'U')
}

function initials(value: string, fallback: string) {
  const parts = value.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return fallback
  }
  if (parts.length === 1 && parts[0].includes('@')) {
    return parts[0][0]?.toUpperCase() ?? fallback
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
