import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Outlet, createFileRoute, useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon, UserMultipleIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgMembersQueryOptions } from '#/lib/api/query'
import type { OrgMember } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { Badge } from '#/components/ui/badge'
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

export const Route = createFileRoute('/orgs/$org_slug/users')({
  component: OrganizationUsersRoute,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
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
  const queryClient = useQueryClient()
  const [isAddingUser, setIsAddingUser] = useState(false)
  const [email, setEmail] = useState('')
  const [fieldErrors, setFieldErrors] = useState<{ email?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const members = useQuery(orgMembersQueryOptions(orgSlug, query))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canAddUser = hasPermission(effectivePermissions.data?.permissions, permission.orgInvite)
  const data = members.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!members.error) {
      return
    }

    toast.error(members.error instanceof Error ? members.error.message : 'Failed to load users')
  }, [members.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }

    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load user permissions')
  }, [effectivePermissions.error])

  const addUser = useMutation({
    mutationFn: async () =>
      api.post<void>(`/api/v1/orgs/${orgSlug}/members`, {
        email: email.trim(),
      }),
    onSuccess: async () => {
      setIsAddingUser(false)
      resetAddUser()
      toast.success('Done')
      await queryClient.invalidateQueries({ queryKey: ['org-members', orgSlug] })
    },
    onError: (error) => {
      if (isApiError(error)) {
        if (error.status === 404) {
          setIsAddingUser(false)
          resetAddUser()
          toast.success('Done')
          return
        }
        setFieldErrors({
          email: error.fieldErrors?.email,
        })
        if (error.fieldErrors?.email) {
          return
        }
      }

      toast.error(error instanceof Error ? error.message : 'Failed to add user')
    },
  })

  function resetAddUser() {
    setEmail('')
    setFieldErrors({})
  }

  function submitAddUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!email.trim()) {
      setFieldErrors({ email: 'Email is required' })
      return
    }

    setFieldErrors({})
    void addUser.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">Users</h1>
            <p className="text-sm text-muted-foreground">Members of this organization.</p>
          </div>

          {canAddUser ? (
            <Dialog
              open={isAddingUser}
              onOpenChange={(open) => {
                setIsAddingUser(open)
                if (!open) {
                  resetAddUser()
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
                <form className="mt-6 flex flex-col gap-4" onSubmit={submitAddUser}>
                  <div className="flex flex-col gap-2">
                    <Input
                      value={email}
                      onChange={(event) => {
                        setEmail(event.target.value)
                        setFieldErrors((current) => ({ ...current, email: undefined }))
                      }}
                      placeholder="Email"
                      aria-invalid={fieldErrors.email ? true : undefined}
                      disabled={addUser.isPending}
                    />
                    {fieldErrors.email ? <p className="text-sm text-destructive">{fieldErrors.email}</p> : null}
                  </div>

                  <DialogFooter>
                    <DialogClose render={<Button type="button" variant="ghost" disabled={addUser.isPending} />}>
                      Cancel
                    </DialogClose>
                    <Button type="submit" disabled={addUser.isPending}>
                      {addUser.isPending ? 'Adding...' : 'Add User'}
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
          placeholder="Search users"
        />
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
                  <TableColumnHeader label="Role" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Joined" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.isLoading ? <UsersTableSkeleton /> : null}
              {members.isError ? <TableEmptyState colSpan={3} icon={UserMultipleIcon} message="Failed to load users." /> : null}
              {!members.isLoading && !members.isError && items.length === 0 ? (
                <TableEmptyState colSpan={3} icon={UserMultipleIcon} message={query.q ? 'No users matched your search.' : 'No users found.'} />
              ) : null}
              {!members.isLoading && !members.isError
                ? items.map((member) => <UserRow key={member.account_id} member={member} />)
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!members.isLoading && !members.isError && items.length > 0 ? (
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
          <InitialsAvatar value={member.name || member.email} />
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{member.name || member.email}</div>
            <div className="truncate text-muted-foreground">{member.email}</div>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant={roleBadgeVariant(member.role)}>{roleLabel(member.role)}</Badge>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(member.joined_at)}</TableCell>
    </TableRow>
  )
}

function UsersTableSkeleton() {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
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
            <Skeleton className="h-5 w-16 rounded-full" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
        </TableRow>
      ))}
    </>
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

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
