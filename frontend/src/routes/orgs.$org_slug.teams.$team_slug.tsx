import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon, UserGroupIcon, UserMultipleIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { orgEffectivePermissionsQueryOptions, orgMembersQueryOptions, orgTeamMembersQueryOptions, orgTeamQueryOptions } from '#/lib/api/query'
import type { OrgMember, TeamMember } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
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

export const Route = createFileRoute('/orgs/$org_slug/teams/$team_slug')({
  component: OrganizationTeamContextPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function OrganizationTeamContextPage() {
  const { org_slug: orgSlug, team_slug: teamSlug } = Route.useParams()
  const queryClient = useQueryClient()
  const [isAddingMember, setIsAddingMember] = useState(false)
  const { searchText, setSearchText, debouncedQuery: memberSearch, clearSearch } = useDebouncedQueryText()
  const { query: membersQuery, toggleSort: toggleMembersSort } = useListPageState({
    page: 1,
    page_size: 25,
    sort: 'created_at',
    order: 'asc',
  })
  const team = useQuery(orgTeamQueryOptions(orgSlug, teamSlug))
  const members = useQuery(orgTeamMembersQueryOptions(orgSlug, teamSlug, membersQuery))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const orgMembers = useQuery({
    ...orgMembersQueryOptions(orgSlug, {
      page: 1,
      page_size: 8,
      sort: 'name',
      order: 'asc',
      q: memberSearch,
    }),
    enabled: isAddingMember,
  })
  const memberItems = members.data?.items ?? []
  const existingMemberIDs = new Set(memberItems.map((member) => member.account_id))
  const displayName = team.data?.name ?? teamSlug
  const canManageMembers = hasPermission(effectivePermissions.data?.permissions, permission.orgWrite)

  useEffect(() => {
    if (!team.error) {
      return
    }
    toast.error(team.error instanceof Error ? team.error.message : 'Failed to load team')
  }, [team.error])

  useEffect(() => {
    if (!members.error) {
      return
    }
    toast.error(members.error instanceof Error ? members.error.message : 'Failed to load team members')
  }, [members.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load team permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!orgMembers.error) {
      return
    }
    toast.error(orgMembers.error instanceof Error ? orgMembers.error.message : 'Failed to load users')
  }, [orgMembers.error])

  const addMember = useMutation({
    mutationFn: async (accountID: number) =>
      api.post<void>(`/api/v1/orgs/${orgSlug}/teams/${teamSlug}/members`, {
        account_id: accountID,
      }),
    onSuccess: async (_, accountID) => {
      toast.success('Done')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['org-team-members', orgSlug, teamSlug] }),
        queryClient.invalidateQueries({ queryKey: ['org-member-teams', orgSlug, accountID] }),
      ])
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to add user')
    },
  })

  const removeMember = useMutation({
    mutationFn: async (accountID: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/teams/${teamSlug}/members/${accountID}`),
    onSuccess: async (_, accountID) => {
      toast.success('Done')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['org-team-members', orgSlug, teamSlug] }),
        queryClient.invalidateQueries({ queryKey: ['org-member-teams', orgSlug, accountID] }),
      ])
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to remove user')
    },
  })

  function resetAddMember() {
    clearSearch()
  }

  return (
    <div className="flex flex-col gap-8">
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink render={<Link to="/orgs/$org_slug/teams" params={{ org_slug: orgSlug }} />}>
              Teams
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
          {team.isLoading ? (
            <div className="flex items-center gap-4">
              <Skeleton className="size-12 rounded-full" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-5 w-40" />
                <Skeleton className="h-4 w-28" />
              </div>
            </div>
          ) : null}

          {team.isError ? (
            <ContextMessage icon={UserGroupIcon} message="Failed to load team." />
          ) : null}

          {team.data ? (
            <div className="flex flex-col gap-6">
              <div className="flex items-start gap-4">
                <InitialsAvatar value={team.data.name} fallback="T" size="lg" />
                <div className="min-w-0 flex-1">
                  <h1 className="truncate text-2xl font-semibold tracking-tight">{team.data.name}</h1>
                  <p className="truncate text-sm text-muted-foreground">@{team.data.slug}</p>
                </div>
              </div>

              <div className="grid gap-4 sm:grid-cols-3">
                <InfoBlock label="Team ID" value={String(team.data.id)} />
                <InfoBlock label="Created" value={formatDate(team.data.created_at)} />
                <InfoBlock label="Members" value={String(members.data?.total ?? 0)} />
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Members</CardTitle>
          {canManageMembers ? (
            <Dialog
              open={isAddingMember}
              onOpenChange={(open) => {
                setIsAddingMember(open)
                if (!open) {
                  resetAddMember()
                }
              }}
            >
              <DialogTrigger render={<Button />}>
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
                Add User
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Add User</DialogTitle>
                </DialogHeader>
                <div className="mt-6 flex flex-col gap-4">
                  <SearchInput
                    value={searchText}
                    onValueChange={setSearchText}
                    onClear={clearSearch}
                    placeholder="Search users"
                    className="max-w-none"
                  />

                  <div className="min-h-64">
                    <Table>
                      <TableBody>
                        {orgMembers.isLoading ? <OrgMembersPickerSkeleton /> : null}
                        {orgMembers.isError ? <MessageRow colSpan={2} icon={UserMultipleIcon} message="Failed to load users." /> : null}
                        {!orgMembers.isLoading && !orgMembers.isError && (orgMembers.data?.items ?? []).length === 0 ? (
                          <MessageRow colSpan={2} icon={UserMultipleIcon} message="No users found." />
                        ) : null}
                        {!orgMembers.isLoading && !orgMembers.isError
                          ? (orgMembers.data?.items ?? []).map((member) => (
                              <OrgMemberPickerRow
                                key={member.account_id}
                                member={member}
                                isExistingMember={existingMemberIDs.has(member.account_id)}
                                isPending={addMember.isPending}
                                onAdd={(accountID) => addMember.mutate(accountID)}
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
                  <TableColumnHeader label="User" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Added" sort="created_at" currentSort={membersQuery.sort} currentOrder={membersQuery.order} onSortChange={toggleMembersSort} />
                </TableHead>
                {canManageMembers ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.isLoading ? <MembersTableSkeleton /> : null}
              {members.isError ? <MessageRow colSpan={canManageMembers ? 3 : 2} icon={UserMultipleIcon} message="Failed to load members." /> : null}
              {!members.isLoading && !members.isError && memberItems.length === 0 ? (
                <MessageRow colSpan={canManageMembers ? 3 : 2} icon={UserMultipleIcon} message="This team does not have any members." />
              ) : null}
              {!members.isLoading && !members.isError
                ? memberItems.map((member) => (
                    <MemberRow
                      key={member.account_id}
                      orgSlug={orgSlug}
                      member={member}
                      canManageMembers={canManageMembers}
                      isRemoving={removeMember.isPending}
                      onRemove={(accountID) => removeMember.mutate(accountID)}
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

function MemberRow({
  orgSlug,
  member,
  canManageMembers,
  isRemoving,
  onRemove,
}: {
  orgSlug: string
  member: TeamMember
  canManageMembers: boolean
  isRemoving: boolean
  onRemove: (accountID: number) => void
}) {
  const displayName = member.name || member.email

  return (
    <TableRow>
      <TableCell>
        <Link
          to="/orgs/$org_slug/users/$account_id"
          params={{ org_slug: orgSlug, account_id: String(member.account_id) }}
          className="flex min-w-0 items-center gap-3"
        >
          <InitialsAvatar value={displayName} />
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{displayName}</div>
            <div className="truncate text-muted-foreground">{member.email}</div>
          </div>
        </Link>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(member.created_at)}</TableCell>
      {canManageMembers ? (
        <TableCell className="text-end">
          <Button
            variant="ghost"
            disabled={isRemoving}
            onClick={() => onRemove(member.account_id)}
          >
            Remove
          </Button>
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function OrgMemberPickerRow({
  member,
  isExistingMember,
  isPending,
  onAdd,
}: {
  member: OrgMember
  isExistingMember: boolean
  isPending: boolean
  onAdd: (accountID: number) => void
}) {
  const displayName = member.name || member.email

  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <InitialsAvatar value={displayName} />
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{displayName}</div>
            <div className="truncate text-muted-foreground">{member.email}</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-end">
        <Button
          variant={isExistingMember ? 'outline' : 'default'}
          disabled={isExistingMember || isPending}
          onClick={() => onAdd(member.account_id)}
        >
          {isExistingMember ? 'Added' : 'Add'}
        </Button>
      </TableCell>
    </TableRow>
  )
}

function OrgMembersPickerSkeleton() {
  return (
    <>
      {Array.from({ length: 4 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-full" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-48" />
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

function MembersTableSkeleton() {
  return (
    <>
      {Array.from({ length: 3 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-full" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-48" />
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

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return dateFormatter.format(date)
}
