import { errorMessage } from '#/lib/api/errors'
import { useOrganizationPageTitle } from '#/lib/page-title'
import { trimTrailingSlash } from '#/lib/utils'
import { formatDate } from '#/lib/format'
import { useEffect, useState } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Outlet,
  createFileRoute,
  useNavigate,
  useRouter,
  useRouterState,
} from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { api } from '#/lib/api/client'
import {
  orgEffectivePermissionsQueryOptions,
  orgInvitationsQueryOptions,
  orgMembersQueryOptions,
} from '#/lib/api/query'
import type {
  OrganizationInvitation,
  OrganizationInvitationMutationResponse,
  OrgMember,
} from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { UserAvatar } from '#/components/UserAvatar'
import { SectionTabNav } from '#/components/SectionTabNav'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '#/components/ui/dialog'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'
import { Skeleton } from '#/components/ui/skeleton'
import { Input } from '#/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'

export const Route = createFileRoute('/orgs/$org_slug/users')({
  component: OrganizationUsersRoute,
  pendingComponent: RoutePending,
})

function OrganizationUsersRoute() {
  const { org_slug: orgSlug } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const listPath = `/orgs/${orgSlug}/users`

  if (trimTrailingSlash(pathname) !== listPath) {
    return <Outlet />
  }

  return <OrganizationUsersPage orgSlug={orgSlug} />
}

function OrganizationUsersPage({ orgSlug }: { orgSlug: string }) {
  useOrganizationPageTitle('Users')
  const queryClient = useQueryClient()
  const setupStatus = useSetupStatus()
  const [isAddingUser, setIsAddingUser] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteURL, setInviteURL] = useState('')
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } =
    useListPageState({
      page: 1,
      page_size: 10,
      sort: 'name',
      order: 'asc',
      q: '',
    })
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canReadUsers = hasPermission(effectivePermissions.data?.permissions, permission.orgRead)
  const canAddUser =
    setupStatus.data?.access_mode === 'multi_user' &&
    canReadUsers &&
    hasPermission(effectivePermissions.data?.permissions, permission.orgInvite)
  const members = useQuery({
    ...orgMembersQueryOptions(orgSlug, query),
    enabled: canReadUsers,
  })
  const invitations = useQuery({
    ...orgInvitationsQueryOptions(orgSlug, { page: 1, page_size: 100 }),
    enabled: canAddUser,
  })
  const data = members.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!canReadUsers || !members.error) return
    toast.error(errorMessage(members.error, 'Failed to load users'))
  }, [canReadUsers, members.error])

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(errorMessage(effectivePermissions.error, 'Failed to load user permissions'))
  }, [effectivePermissions.error])

  const inviteUser = useMutation({
    mutationFn: async () =>
      api.post<OrganizationInvitationMutationResponse>(`/api/v1/orgs/${orgSlug}/invitations`, {
        email: inviteEmail.trim(),
      }),
    onSuccess: async (payload) => {
      setInviteURL(payload.invite_url)
      toast.success(payload.delivery_status === 'sent' ? 'Invitation sent' : 'Invitation created')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgInvitationsScope(orgSlug) })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to invite user'))
    },
  })

  function resetAddUser() {
    setInviteEmail('')
    setInviteURL('')
    inviteUser.reset()
  }

  return (
    <div className="flex flex-col">
      <SectionTabNav
        tabs={[
          {
            label: 'Users',
            to: '/orgs/$org_slug/users',
            params: { org_slug: orgSlug },
            isActive: true,
          },
          {
            label: 'Teams',
            to: '/orgs/$org_slug/teams',
            params: { org_slug: orgSlug },
            isActive: false,
          },
        ]}
      />

      <div className="flex flex-col gap-6 pt-6">
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex flex-col gap-1.5">
              <h1 className="font-heading text-2xl font-semibold tracking-tight">Users</h1>
              <p className="text-sm text-muted-foreground">
                {!members.isLoading && total > 0
                  ? `${total} member${total !== 1 ? 's' : ''} in this organization`
                  : 'Members of this organization.'}
              </p>
            </div>

            {canAddUser ? (
              <Dialog
                open={isAddingUser}
                onOpenChange={(open) => {
                  setIsAddingUser(open)
                  if (!open) resetAddUser()
                }}
              >
                <DialogTrigger render={<Button />}>
                  <Icon name="plus-sign" size={20} data-icon="inline-start" />
                  Invite User
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Invite User</DialogTitle>
                  </DialogHeader>
                  <div className="mt-6 flex flex-col gap-4">
                    <div className="flex flex-col gap-2">
                      <label htmlFor="invite-email" className="text-sm font-medium">
                        Email address
                      </label>
                      <Input
                        id="invite-email"
                        type="email"
                        autoComplete="email"
                        value={inviteEmail}
                        disabled={inviteUser.isPending || Boolean(inviteURL)}
                        onChange={(event) => setInviteEmail(event.target.value)}
                        placeholder="person@example.com"
                      />
                      <p className="text-sm text-muted-foreground">
                        They will receive baseline organization access after accepting.
                      </p>
                    </div>
                    {inviteURL ? (
                      <div className="flex flex-col gap-2">
                        <label htmlFor="invite-link" className="text-sm font-medium">
                          Invitation link
                        </label>
                        <div className="flex gap-2">
                          <Input id="invite-link" readOnly value={inviteURL} />
                          <Button
                            type="button"
                            variant="outline"
                            onClick={() => {
                              copyText(inviteURL)
                              toast.success('Invitation link copied')
                            }}
                          >
                            Copy
                          </Button>
                        </div>
                      </div>
                    ) : null}

                    <DialogFooter>
                      <DialogClose
                        render={
                          <Button type="button" variant="ghost" disabled={inviteUser.isPending} />
                        }
                      >
                        {inviteURL ? 'Done' : 'Cancel'}
                      </DialogClose>
                      {!inviteURL ? (
                        <Button
                          type="button"
                          disabled={!inviteEmail.trim() || inviteUser.isPending}
                          onClick={() => inviteUser.mutate()}
                        >
                          {inviteUser.isPending ? 'Creating…' : 'Send invitation'}
                        </Button>
                      ) : null}
                    </DialogFooter>
                  </div>
                </DialogContent>
              </Dialog>
            ) : null}
          </div>

          <SearchInput
            value={searchText}
            onValueChange={setSearchText}
            onClear={clearSearch}
            placeholder="Search users"
          />
        </div>

        <Card>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    <TableColumnHeader
                      label="User"
                      sort="name"
                      currentSort={query.sort}
                      currentOrder={query.order}
                      onSortChange={toggleSort}
                    />
                  </TableHead>
                  <TableHead>
                    <TableColumnHeader
                      label="Joined"
                      sort="created_at"
                      currentSort={query.sort}
                      currentOrder={query.order}
                      onSortChange={toggleSort}
                    />
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {effectivePermissions.isLoading || members.isLoading ? (
                  <UsersTableSkeleton />
                ) : null}
                {members.isError ? (
                  <TableEmptyState
                    colSpan={2}
                    icon="user-multiple"
                    message="Failed to load users."
                  />
                ) : null}
                {!effectivePermissions.isLoading && !canReadUsers ? (
                  <TableEmptyState
                    colSpan={2}
                    icon="user-multiple"
                    message="You do not have permission to view users."
                  />
                ) : null}
                {!effectivePermissions.isLoading &&
                canReadUsers &&
                !members.isLoading &&
                !members.isError &&
                items.length === 0 ? (
                  <TableEmptyState
                    colSpan={2}
                    icon="user-multiple"
                    message={query.q ? 'No users matched your search.' : 'No users found.'}
                  />
                ) : null}
                {!effectivePermissions.isLoading &&
                canReadUsers &&
                !members.isLoading &&
                !members.isError
                  ? items.map((member) => <UserRow key={member.account_id} member={member} />)
                  : null}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {canReadUsers && !members.isLoading && !members.isError && items.length > 0 ? (
          <PaginationFooter
            itemLabel="users"
            page={page}
            pageCount={pageCount}
            pageSize={pageSize}
            total={total}
            isFetching={members.isFetching}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        ) : null}

        {canAddUser ? (
          <PendingInvitations
            orgSlug={orgSlug}
            invitations={invitations.data?.items ?? []}
            isLoading={invitations.isLoading}
            isError={invitations.isError}
          />
        ) : null}
      </div>
    </div>
  )
}

function UserRow({ member }: { member: OrgMember }) {
  const { org_slug: orgSlug } = Route.useParams()
  const router = useRouter()
  const navigate = useNavigate()

  function preloadUser() {
    void router.preloadRoute({
      to: '/orgs/$org_slug/users/$account_id',
      params: { org_slug: orgSlug, account_id: String(member.account_id) },
    })
  }

  function openUser() {
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
      onFocus={preloadUser}
      onMouseEnter={preloadUser}
      onClick={openUser}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          openUser()
        }
      }}
    >
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <UserAvatar value={member.name || member.email} fallback="?" />
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate font-medium text-foreground">
                {member.name || member.email}
              </span>
              {member.role ? (
                <Badge variant="outline" className="h-4 shrink-0 px-1.5 py-0 text-[10px]">
                  {elevationLabel(member.role)}
                </Badge>
              ) : null}
            </div>
            <div className="truncate text-sm text-muted-foreground">{member.email}</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(member.joined_at)}</TableCell>
    </TableRow>
  )
}

function PendingInvitations({
  orgSlug,
  invitations,
  isLoading,
  isError,
}: {
  orgSlug: string
  invitations: OrganizationInvitation[]
  isLoading: boolean
  isError: boolean
}) {
  const queryClient = useQueryClient()
  const resend = useMutation({
    mutationFn: (id: string) =>
      api.post<OrganizationInvitationMutationResponse>(
        `/api/v1/orgs/${orgSlug}/invitations/${id}/resend`,
      ),
    onSuccess: async (payload) => {
      if (payload.delivery_status !== 'sent') {
        copyText(payload.invite_url)
        toast.success('Invitation renewed and link copied')
      } else {
        toast.success('Invitation resent')
      }
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgInvitationsScope(orgSlug) })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to resend invitation')),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => api.delete<void>(`/api/v1/orgs/${orgSlug}/invitations/${id}`),
    onSuccess: async () => {
      toast.success('Invitation revoked')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgInvitationsScope(orgSlug) })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to revoke invitation')),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Pending invitations</CardTitle>
        <CardDescription>
          Invitations expire after seven days and grant baseline organization access.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Email</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={4}>
                  <Skeleton className="h-5 w-48" />
                </TableCell>
              </TableRow>
            ) : null}
            {isError ? (
              <TableEmptyState
                colSpan={4}
                icon="user-multiple"
                message="Failed to load invitations."
              />
            ) : null}
            {!isLoading && !isError && invitations.length === 0 ? (
              <TableEmptyState colSpan={4} icon="user-multiple" message="No pending invitations." />
            ) : null}
            {!isError
              ? invitations.map((invitation) => (
                  <TableRow key={invitation.id}>
                    <TableCell className="font-medium">{invitation.email}</TableCell>
                    <TableCell>
                      <Badge variant={invitation.status === 'expired' ? 'destructive' : 'outline'}>
                        {invitation.status === 'expired' ? 'Expired' : 'Pending'}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatDate(invitation.expires_at)}
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={resend.isPending || revoke.isPending}
                          onClick={() => resend.mutate(invitation.id)}
                        >
                          Resend
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          disabled={resend.isPending || revoke.isPending}
                          onClick={() => revoke.mutate(invitation.id)}
                        >
                          Revoke
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              : null}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function UsersTableSkeleton() {
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
        </TableRow>
      ))}
    </>
  )
}

function elevationLabel(role: string) {
  if (role.includes('owner')) return 'Owner'
  if (role.includes('admin')) return 'Admin'
  return role
}

function copyText(value: string) {
  if (navigator.clipboard) {
    void navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.style.cssText = 'position:fixed;opacity:0'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  textarea.remove()
}
