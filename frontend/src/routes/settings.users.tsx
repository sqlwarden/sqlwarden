import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon } from '@hugeicons/core-free-icons'
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
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { InitialsAvatar } from '#/components/InitialsAvatar'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/settings/users')({
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
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [values, setValues] = useState<CreateUserValues>({
    name: '',
    email: '',
    password: '',
    confirmPassword: '',
  })
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<keyof CreateUserValues, string>>>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
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
    toast.error(users.error instanceof Error ? users.error.message : 'Failed to load users')
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
      await queryClient.invalidateQueries({ queryKey: ['instance-accounts'] })
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
      toast.error(error instanceof Error ? error.message : 'Failed to create user')
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
    if (!values.name.trim()) nextErrors.name = 'Name is required'
    if (!values.email.trim()) nextErrors.email = 'Email is required'
    if (!values.password) nextErrors.password = 'Password is required'
    else if (values.password.length < 8) nextErrors.password = 'Password must be at least 8 characters'
    if (!values.confirmPassword) nextErrors.confirmPassword = 'Confirm the password'
    else if (values.password !== values.confirmPassword) nextErrors.confirmPassword = 'Passwords do not match'
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
            <h2 className="text-2xl font-semibold tracking-tight">Users</h2>
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
              <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
              Create
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create user</DialogTitle>
                <DialogDescription>Create a local account. Organization membership is managed from organization user pages.</DialogDescription>
              </DialogHeader>
              <form className="mt-6 flex flex-col gap-4" onSubmit={submitCreateUser}>
                <FormInput
                  value={values.name}
                  error={fieldErrors.name}
                  placeholder="Full name"
                  autoComplete="name"
                  disabled={createUser.isPending}
                  onChange={(value) => updateField('name', value)}
                />
                <FormInput
                  type="email"
                  value={values.email}
                  error={fieldErrors.email}
                  placeholder="user@example.com"
                  autoComplete="email"
                  disabled={createUser.isPending}
                  onChange={(value) => updateField('email', value)}
                />
                <FormInput
                  type="password"
                  value={values.password}
                  error={fieldErrors.password}
                  placeholder="Temporary password"
                  autoComplete="new-password"
                  disabled={createUser.isPending}
                  onChange={(value) => updateField('password', value)}
                />
                <FormInput
                  type="password"
                  value={values.confirmPassword}
                  error={fieldErrors.confirmPassword}
                  placeholder="Confirm password"
                  autoComplete="new-password"
                  disabled={createUser.isPending}
                  onChange={(value) => updateField('confirmPassword', value)}
                />

                <DialogFooter>
                  <DialogClose render={<Button type="button" variant="ghost" disabled={createUser.isPending} />}>
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
                  <TableColumnHeader label="User" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Status" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Account ID" sort="id" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.isLoading ? <TableEmptyState colSpan={4} compact message="Loading users..." /> : null}
              {users.isError ? <TableEmptyState colSpan={4} compact message="Failed to load users." /> : null}
              {!users.isLoading && !users.isError && items.length === 0 ? (
                <TableEmptyState colSpan={4} compact message={query.q ? 'No users matched your search.' : 'No users exist yet.'} />
              ) : null}
              {items.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="flex min-w-0 items-center gap-3">
                      <InitialsAvatar value={user.name || user.email} />
                      <div className="min-w-0">
                        <div className="truncate font-medium text-foreground">{user.name}</div>
                        <div className="truncate text-muted-foreground">{user.email}</div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={user.is_active ? 'secondary' : 'outline'}>{user.is_active ? 'Active' : 'Inactive'}</Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{user.id}</TableCell>
                  <TableCell className="text-muted-foreground">{formatDate(user.created_at)}</TableCell>
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

function FormInput({
  type = 'text',
  value,
  error,
  placeholder,
  autoComplete,
  disabled,
  onChange,
}: {
  type?: string
  value: string
  error?: string
  placeholder: string
  autoComplete: string
  disabled: boolean
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-2">
      <Input
        type={type}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        aria-invalid={error ? true : undefined}
        disabled={disabled}
      />
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}

function formatDate(value?: string) {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Unknown'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date)
}
