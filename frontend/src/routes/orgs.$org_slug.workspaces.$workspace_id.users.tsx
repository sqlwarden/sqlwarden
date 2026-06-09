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
  orgMembersQueryOptions,
  orgWorkspaceEffectiveMembersQueryOptions,
  orgWorkspaceMembersQueryOptions,
} from '#/lib/api/query'
import type { OrgMember, WorkspaceEffectiveMember, WorkspaceMember, WorkspaceMembershipSource } from '#/lib/api/types'
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
import { Checkbox } from '#/components/ui/checkbox'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { getInitials } from '#/components/InitialsAvatar'
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

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/users')({
  component: WorkspaceUsersPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function WorkspaceUsersPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const queryClient = useQueryClient()
  const [isAddingUser, setIsAddingUser] = useState(false)
  const [includeInheritedUsers, setIncludeInheritedUsers] = useState(false)
  const { searchText: pickerSearchText, setSearchText: setPickerSearchText, debouncedQuery: pickerSearch, clearSearch: clearPickerSearch } = useDebouncedQueryText()
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const canReadUsers = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canModifyUsers = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)
  const directMembers = useQuery({
    ...orgWorkspaceMembersQueryOptions(orgSlug, workspaceId, query),
    enabled: canReadUsers,
  })
  const effectiveMembers = useQuery({
    ...orgWorkspaceEffectiveMembersQueryOptions(orgSlug, workspaceId, query),
    enabled: canReadUsers && includeInheritedUsers,
  })
  const orgMembers = useQuery({
    ...orgMembersQueryOptions(orgSlug, {
      page: 1,
      page_size: 10,
      sort: 'name',
      order: 'asc',
      q: pickerSearch,
    }),
    enabled: isAddingUser && canModifyUsers,
  })

  const activeMembers = includeInheritedUsers ? effectiveMembers : directMembers
  const items: WorkspaceUserRowItem[] = includeInheritedUsers
    ? (effectiveMembers.data?.items ?? [])
    : (directMembers.data?.items ?? []).map(workspaceMemberToRowItem)
  const existingAccountIds = new Set((directMembers.data?.items ?? []).map((member) => member.account_id))
  const page = activeMembers.data?.page ?? Number(query.page ?? 1)
  const pageSize = activeMembers.data?.page_size ?? Number(query.page_size ?? 10)
  const total = activeMembers.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1
  const tableColumnCount = 2 + (includeInheritedUsers ? 1 : 0) + (canModifyUsers ? 1 : 0)

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load user permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!canReadUsers || !activeMembers.error) return
    toast.error(activeMembers.error instanceof Error ? activeMembers.error.message : 'Failed to load workspace users')
  }, [canReadUsers, activeMembers.error])

  useEffect(() => {
    if (!isAddingUser || !canModifyUsers || !orgMembers.error) return
    toast.error(orgMembers.error instanceof Error ? orgMembers.error.message : 'Failed to load organization users')
  }, [canModifyUsers, isAddingUser, orgMembers.error])

  const addUser = useMutation({
    mutationFn: async (accountId: number) =>
      api.post<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/users`, { account_id: accountId }),
    onSuccess: async () => {
      toast.success('User added')
      await invalidateWorkspaceUserQueries(queryClient, orgSlug, workspaceId)
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to add user')
    },
  })

  const removeUser = useMutation({
    mutationFn: async (accountId: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/users/${accountId}`),
    onSuccess: async () => {
      toast.success('User removed')
      await invalidateWorkspaceUserQueries(queryClient, orgSlug, workspaceId)
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to remove user')
    },
  })

  return (
    <div className="flex flex-col">
      <SectionTabNav
        tabs={[
          { label: 'Users', to: '/orgs/$org_slug/workspaces/$workspace_id/users', params: { org_slug: orgSlug, workspace_id: workspaceId }, isActive: true },
          { label: 'Teams', to: '/orgs/$org_slug/workspaces/$workspace_id/teams', params: { org_slug: orgSlug, workspace_id: workspaceId }, isActive: false },
        ]}
      />

      <div className="flex flex-col gap-6 pt-6">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            {!activeMembers.isLoading && total > 0
              ? `${total} user${total !== 1 ? 's' : ''} in this workspace`
              : includeInheritedUsers
                ? 'Direct and inherited members of this workspace.'
                : 'Users explicitly added to this workspace.'}
          </p>
          {canModifyUsers ? (
            <Dialog
              open={isAddingUser}
              onOpenChange={(open) => {
                setIsAddingUser(open)
                if (!open) clearPickerSearch()
              }}
            >
              <DialogTrigger render={<Button />}>
                <Icon name="plus-sign" size={20} data-icon="inline-start" />
                Add User
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Add User</DialogTitle>
                </DialogHeader>
                <div className="mt-6 flex flex-col gap-4">
                  <SearchInput
                    value={pickerSearchText}
                    onValueChange={setPickerSearchText}
                    onClear={clearPickerSearch}
                    placeholder="Search organization users"
                    className="max-w-none"
                  />
                  <div className="min-h-64">
                    <Table>
                      <TableBody>
                        {orgMembers.isLoading ? <UserPickerSkeleton /> : null}
                        {orgMembers.isError ? <MessageRow colSpan={2} icon="user-multiple" message="Failed to load users." /> : null}
                        {!orgMembers.isLoading && !orgMembers.isError && (orgMembers.data?.items ?? []).length === 0 ? (
                          <MessageRow colSpan={2} icon="user-multiple" message="No users found." />
                        ) : null}
                        {!orgMembers.isLoading && !orgMembers.isError
                          ? (orgMembers.data?.items ?? []).map((member) => (
                              <UserPickerRow
                                key={member.account_id}
                                member={member}
                                isExistingMember={existingAccountIds.has(member.account_id)}
                                isPending={addUser.isPending}
                                onAdd={(accountId) => addUser.mutate(accountId)}
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

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <SearchInput
            value={searchText}
            onValueChange={setSearchText}
            onClear={clearSearch}
            placeholder="Search users"
          />
          <label className="flex shrink-0 cursor-pointer items-center gap-2 text-sm text-muted-foreground">
            <Checkbox
              checked={includeInheritedUsers}
              onCheckedChange={(checked) => {
                setIncludeInheritedUsers(checked === true)
                setPage(1)
              }}
            />
            Show inherited users
          </label>
        </div>
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="User" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Added" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                {includeInheritedUsers ? (
                  <TableHead>
                    <TableColumnHeader label="Source" />
                  </TableHead>
                ) : null}
                {canModifyUsers ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectivePermissions.isLoading || activeMembers.isLoading ? (
                <UsersTableSkeleton canModifyUsers={canModifyUsers} includeSource={includeInheritedUsers} />
              ) : null}
              {activeMembers.isError ? (
                <TableEmptyState colSpan={tableColumnCount} icon="user-multiple" message="Failed to load users." />
              ) : null}
              {!effectivePermissions.isLoading && !canReadUsers ? (
                <TableEmptyState colSpan={tableColumnCount} icon="user-multiple" message="You do not have permission to view workspace users." />
              ) : null}
              {!effectivePermissions.isLoading && canReadUsers && !activeMembers.isLoading && !activeMembers.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={tableColumnCount}
                  icon="user-multiple"
                  message={
                    query.q
                      ? 'No users matched your search.'
                      : includeInheritedUsers
                        ? 'No direct or inherited workspace users found.'
                        : 'No workspace users found.'
                  }
                />
              ) : null}
              {!effectivePermissions.isLoading && canReadUsers && !activeMembers.isLoading && !activeMembers.isError
                ? items.map((member) => (
                    <WorkspaceUserRow
                      key={member.account_id}
                      orgSlug={orgSlug}
                      member={member}
                      canModifyUsers={canModifyUsers}
                      showSource={includeInheritedUsers}
                      isRemoving={removeUser.isPending}
                      onRemove={(accountId) => removeUser.mutate(accountId)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {canReadUsers && !activeMembers.isLoading && !activeMembers.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="users"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={activeMembers.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}
      </div>
    </div>
  )
}

function UserPickerRow({
  member,
  isExistingMember,
  isPending,
  onAdd,
}: {
  member: OrgMember
  isExistingMember: boolean
  isPending: boolean
  onAdd: (accountId: number) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold', entityColor(member.name || member.email))}>
            {getInitials(member.name || member.email, '?')}
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{member.name || member.email}</div>
            <div className="truncate text-sm text-muted-foreground">{member.email}</div>
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

function WorkspaceUserRow({
  orgSlug,
  member,
  canModifyUsers,
  showSource,
  isRemoving,
  onRemove,
}: {
  orgSlug: string
  member: WorkspaceUserRowItem
  canModifyUsers: boolean
  showSource: boolean
  isRemoving: boolean
  onRemove: (accountId: number) => void
}) {
  const router = useRouter()
  const navigate = useNavigate()

  function preload() {
    void router.preloadRoute({
      to: '/orgs/$org_slug/users/$account_id',
      params: { org_slug: orgSlug, account_id: String(member.account_id) },
    })
  }

  function open() {
    void navigate({
      to: '/orgs/$org_slug/users/$account_id',
      params: { org_slug: orgSlug, account_id: String(member.account_id) },
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
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold', entityColor(member.name || member.email))}>
            {getInitials(member.name || member.email, '?')}
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{member.name || member.email}</div>
            <div className="truncate text-sm text-muted-foreground">{member.email}</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(member.created_at)}</TableCell>
      {showSource ? (
        <TableCell>
          <MembershipSourceBadges sources={member.membership_sources} />
        </TableCell>
      ) : null}
      {canModifyUsers ? (
        <TableCell className="text-end" onClick={(e) => e.stopPropagation()}>
          {member.is_direct_member ? (
            <AlertDialog>
              <AlertDialogTrigger render={<Button variant="destructive" disabled={isRemoving} />}>
                Remove
              </AlertDialogTrigger>
              <AlertDialogContent size="sm">
                <AlertDialogHeader>
                  <AlertDialogTitle>Remove user from workspace?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This removes {member.name || member.email} from direct workspace membership.
                    {member.membership_sources.some((s) => s.type === 'team')
                      ? ' The user may remain visible through inherited team membership.'
                      : ' Permissions granted through workspace membership will no longer apply.'}
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel variant="ghost" disabled={isRemoving}>Cancel</AlertDialogCancel>
                  <AlertDialogAction variant="destructive" disabled={isRemoving} onClick={() => onRemove(member.account_id)}>
                    Remove
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">Inherited</Badge>
          )}
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function MembershipSourceBadges({ sources }: { sources: WorkspaceMembershipSource[] }) {
  const teamSources = sources.filter((s) => s.type === 'team')
  const hasDirectSource = sources.some((s) => s.type === 'direct')

  return (
    <div className="flex flex-wrap gap-1.5">
      {hasDirectSource ? <Badge variant="outline">Direct</Badge> : null}
      {teamSources.map((source) => (
        <Badge key={`${source.team_id ?? source.team_slug}-${source.team_name ?? 'team'}`} variant="outline">
          {source.team_name || source.team_slug || 'Team'}
        </Badge>
      ))}
    </div>
  )
}

function UserPickerSkeleton() {
  return (
    <>
      {Array.from({ length: 4 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-md" />
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

function UsersTableSkeleton({ canModifyUsers, includeSource }: { canModifyUsers: boolean; includeSource: boolean }) {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-md" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-48" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          {includeSource ? (
            <TableCell>
              <Skeleton className="h-5 w-20 rounded-full" />
            </TableCell>
          ) : null}
          {canModifyUsers ? (
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

type WorkspaceUserRowItem = WorkspaceEffectiveMember

function workspaceMemberToRowItem(member: WorkspaceMember): WorkspaceUserRowItem {
  return {
    ...member,
    is_direct_member: true,
    membership_sources: [{ type: 'direct', created_at: member.created_at }],
  }
}

async function invalidateWorkspaceUserQueries(queryClient: ReturnType<typeof useQueryClient>, orgSlug: string, workspaceId: string) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['org-workspace-members', orgSlug, workspaceId] }),
    queryClient.invalidateQueries({ queryKey: ['org-workspace-effective-members', orgSlug, workspaceId] }),
  ])
}
