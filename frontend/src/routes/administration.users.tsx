import { errorMessage } from '#/lib/api/errors'
import { formatDate } from '#/lib/format'
import { useEffect, useState } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { instanceAccountsQueryOptions } from '#/lib/api/query'
import type { Account } from '#/lib/api/types'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '#/components/ui/dialog'
import { FormField } from '#/components/ui/field'
import { Input } from '#/components/ui/input'
import { UserAvatar } from '#/components/UserAvatar'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { Skeleton } from '#/components/ui/skeleton'
import { usePageTitle } from '#/lib/page-title'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'

export const Route = createFileRoute('/administration/users')({
  component: SettingsUsersPage,
  pendingComponent: RoutePending,
})

type CreateUserValues = {
  name: string
  email: string
  password: string
  confirmPassword: string
}

function SettingsUsersPage() {
  usePageTitle('Users', 'Administration')
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [values, setValues] = useState<CreateUserValues>({
    name: '',
    email: '',
    password: '',
    confirmPassword: '',
  })
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<keyof CreateUserValues, string>>>(
    {},
  )
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } =
    useListPageState({
      page: 1,
      page_size: 10,
      sort: 'created_at',
      order: 'desc',
      q: '',
    })

  const users = useQuery(instanceAccountsQueryOptions(query))
  const items = users.data?.items ?? []
  const page = users.data?.page ?? Number(query.page ?? 1)
  const pageSize = users.data?.page_size ?? Number(query.page_size ?? 10)
  const total = users.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!users.error) return
    toast.error(errorMessage(users.error, 'Failed to load users'))
  }, [users.error])

  const createUser = useMutation({
    mutationFn: async () =>
      api.post<Account>('/api/v1/instance/accounts', {
        name: values.name.trim(),
        email: values.email.trim(),
        password: values.password,
      }),
    onSuccess: async () => {
      setIsCreating(false)
      resetCreateUser()
      toast.success('User created')
      await queryClient.invalidateQueries({ queryKey: queryKeys.instanceAccountsScope() })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          name: error.fieldErrors?.name,
          email: error.fieldErrors?.email,
          password: error.fieldErrors?.password,
        })
        if (error.fieldErrors?.name || error.fieldErrors?.email || error.fieldErrors?.password) {
          return
        }
      }
      toast.error(errorMessage(error, 'Failed to create user'))
    },
  })

  function resetCreateUser() {
    setValues({
      name: '',
      email: '',
      password: '',
      confirmPassword: '',
    })
    setFieldErrors({})
  }

  function updateField(field: keyof CreateUserValues, value: string) {
    setValues((current) => ({ ...current, [field]: value }))
    setFieldErrors((current) => ({ ...current, [field]: undefined }))
    if (field === 'password' || field === 'confirmPassword') {
      setFieldErrors((current) => ({ ...current, password: undefined, confirmPassword: undefined }))
    }
  }

  function submitCreateUser(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const nextErrors: Partial<Record<keyof CreateUserValues, string>> = {}
    if (!values.name.trim()) nextErrors.name = 'Name is required.'
    if (!values.email.trim()) nextErrors.email = 'Email is required.'
    if (!values.password) nextErrors.password = 'Password is required.'
    else if (values.password.length < 8)
      nextErrors.password = 'Password must be at least 8 characters.'
    if (!values.confirmPassword) nextErrors.confirmPassword = 'Confirm the password.'
    else if (values.password !== values.confirmPassword)
      nextErrors.confirmPassword = 'Passwords do not match.'
    if (Object.keys(nextErrors).length > 0) {
      setFieldErrors(nextErrors)
      return
    }

    setFieldErrors({})
    void createUser.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h2 className="font-heading text-2xl font-semibold tracking-tight">Users</h2>
            <p className="text-sm text-muted-foreground">
              {!users.isLoading && total > 0
                ? `${total} user${total !== 1 ? 's' : ''} on this instance`
                : 'All local users on this instance.'}
            </p>
          </div>
          <Dialog
            open={isCreating}
            onOpenChange={(open) => {
              setIsCreating(open)
              if (!open) resetCreateUser()
            }}
          >
            <DialogTrigger render={<Button />}>
              <Icon name="plus-sign" size={20} data-icon="inline-start" />
              Create
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create user</DialogTitle>
                <DialogDescription>
                  Create a local account. Organization membership is managed from organization user
                  pages.
                </DialogDescription>
              </DialogHeader>
              <form className="mt-6 flex flex-col gap-4" onSubmit={submitCreateUser}>
                <FormField label="Name" htmlFor="create-user-name" error={fieldErrors.name}>
                  <Input
                    id="create-user-name"
                    value={values.name}
                    onChange={(event) => updateField('name', event.target.value)}
                    placeholder="Full name"
                    autoComplete="name"
                    aria-invalid={fieldErrors.name ? true : undefined}
                    disabled={createUser.isPending}
                  />
                </FormField>
                <FormField label="Email" htmlFor="create-user-email" error={fieldErrors.email}>
                  <Input
                    id="create-user-email"
                    type="email"
                    value={values.email}
                    onChange={(event) => updateField('email', event.target.value)}
                    placeholder="user@example.com"
                    autoComplete="email"
                    aria-invalid={fieldErrors.email ? true : undefined}
                    disabled={createUser.isPending}
                  />
                </FormField>
                <FormField
                  label="Password"
                  htmlFor="create-user-password"
                  error={fieldErrors.password}
                >
                  <Input
                    id="create-user-password"
                    type="password"
                    value={values.password}
                    onChange={(event) => updateField('password', event.target.value)}
                    placeholder="Temporary password"
                    autoComplete="new-password"
                    aria-invalid={fieldErrors.password ? true : undefined}
                    disabled={createUser.isPending}
                  />
                </FormField>
                <FormField
                  label="Confirm password"
                  htmlFor="create-user-confirm-password"
                  error={fieldErrors.confirmPassword}
                >
                  <Input
                    id="create-user-confirm-password"
                    type="password"
                    value={values.confirmPassword}
                    onChange={(event) => updateField('confirmPassword', event.target.value)}
                    placeholder="Confirm password"
                    autoComplete="new-password"
                    aria-invalid={fieldErrors.confirmPassword ? true : undefined}
                    disabled={createUser.isPending}
                  />
                </FormField>

                <DialogFooter>
                  <DialogClose
                    render={
                      <Button type="button" variant="ghost" disabled={createUser.isPending} />
                    }
                  >
                    Cancel
                  </DialogClose>
                  <Button type="submit" disabled={createUser.isPending}>
                    {createUser.isPending ? 'Creating...' : 'Create'}
                  </Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
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
                  <TableColumnHeader label="Status" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader
                    label="Account ID"
                    sort="id"
                    currentSort={query.sort}
                    currentOrder={query.order}
                    onSortChange={toggleSort}
                  />
                </TableHead>
                <TableHead>
                  <TableColumnHeader
                    label="Created"
                    sort="created_at"
                    currentSort={query.sort}
                    currentOrder={query.order}
                    onSortChange={toggleSort}
                  />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.isLoading ? <UsersTableSkeleton /> : null}
              {users.isError ? (
                <TableEmptyState colSpan={4} compact message="Failed to load users." />
              ) : null}
              {!users.isLoading && !users.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={4}
                  compact
                  message={query.q ? 'No users matched your search.' : 'No users exist yet.'}
                />
              ) : null}
              {items.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="flex min-w-0 items-center gap-3">
                      <UserAvatar value={user.name || user.email} />
                      <div className="min-w-0">
                        <div className="truncate font-medium text-foreground">{user.name}</div>
                        <div className="truncate text-muted-foreground">{user.email}</div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={user.is_active ? 'secondary' : 'outline'}>
                      {user.is_active ? 'Active' : 'Inactive'}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{user.id}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(user.created_at)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!users.isLoading && !users.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="users"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={users.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}
    </div>
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
                <Skeleton className="h-3 w-40" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-16" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}
