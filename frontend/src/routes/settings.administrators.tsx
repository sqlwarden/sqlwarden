import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { Delete02Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { instanceAdminsQueryOptions, queryKeys } from '#/lib/api/query'
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

export const Route = createFileRoute('/settings/administrators')({
  component: SettingsAdministratorsPage,
  pendingComponent: RoutePending,
})

function SettingsAdministratorsPage() {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [email, setEmail] = useState('')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'asc',
    q: '',
  })

  const administrators = useQuery(instanceAdminsQueryOptions(query))
  const data = administrators.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  const addAdministrator = useMutation({
    mutationFn: async (value: string) => api.post<void>('/api/v1/instance/admins', { email: value }),
    onSuccess: async () => {
      setIsCreating(false)
      setEmail('')
      setFieldError(null)
      toast.success('Administrator added')
      await queryClient.invalidateQueries({ queryKey: ['instance-admins'] })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors?.email) {
        setFieldError(error.fieldErrors.email)
        return
      }

      toast.error(error instanceof Error ? error.message : 'Failed to add administrator')
    },
  })

  const removeAdministrator = useMutation({
    mutationFn: async (accountId: number) => api.delete<void>(`/api/v1/instance/admins/${accountId}`),
    onSuccess: async () => {
      toast.success('Administrator removed')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['instance-admins'] }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to remove administrator')
    },
  })

  useEffect(() => {
    if (!administrators.error) {
      return
    }

    toast.error(administrators.error instanceof Error ? administrators.error.message : 'Failed to load administrators')
  }, [administrators.error])

  function submitCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const value = email.trim()

    if (!value) {
      setFieldError('Email is required')
      return
    }

    setFieldError(null)
    void addAdministrator.mutateAsync(value).catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h2 className="text-2xl font-semibold tracking-tight">Administrators</h2>
            <p className="text-sm text-muted-foreground">
              {!administrators.isLoading && total > 0
                ? `${total} instance administrator${total !== 1 ? 's' : ''}`
                : 'Accounts with full instance access.'}
            </p>
          </div>
          <Dialog
            open={isCreating}
            onOpenChange={(open) => {
              setIsCreating(open)
              if (!open) {
                setEmail('')
                setFieldError(null)
              }
            }}
          >
            <DialogTrigger render={<Button />}>
              <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
              Add
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Add administrator</DialogTitle>
                <DialogDescription>Grant instance administrator access to an existing account by email.</DialogDescription>
              </DialogHeader>
              <form className="mt-6 flex flex-col gap-4" onSubmit={submitCreate}>
                <div className="flex flex-col gap-2">
                  <Input
                    type="email"
                    value={email}
                    onChange={(event) => {
                      setEmail(event.target.value)
                      setFieldError(null)
                    }}
                    placeholder="admin@example.com"
                    aria-invalid={fieldError ? true : undefined}
                    disabled={addAdministrator.isPending}
                  />
                  {fieldError ? <p className="text-sm text-destructive">{fieldError}</p> : null}
                </div>

                <DialogFooter>
                  <DialogClose render={<Button type="button" variant="ghost" disabled={addAdministrator.isPending} />}>
                    Cancel
                  </DialogClose>
                  <Button type="submit" disabled={addAdministrator.isPending}>
                    {addAdministrator.isPending ? 'Adding…' : 'Add'}
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
          placeholder="Search administrators"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Account" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Account ID" sort="account_id" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Added" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead className="w-1 text-right">
                  <TableColumnHeader label="Actions" />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {administrators.isLoading ? (
                <TableEmptyState colSpan={4} compact message="Loading administrators…" />
              ) : null}
              {administrators.isError ? (
                <TableEmptyState colSpan={4} compact message="Failed to load administrators." />
              ) : null}
              {!administrators.isLoading && !administrators.isError && items.length === 0 ? (
                <TableEmptyState colSpan={4} compact message={query.q ? 'No administrators matched your search.' : 'No administrators exist yet.'} />
              ) : null}
              {items.map((administrator) => (
                <TableRow key={administrator.account_id}>
                  <TableCell>
                    <div className="flex min-w-0 items-center gap-3">
                      <InitialsAvatar value={administrator.account?.name || administrator.account?.email || 'A'} />
                      <div className="min-w-0">
                        <div className="truncate font-medium text-foreground">{administrator.account?.name ?? 'Unknown'}</div>
                        <div className="truncate text-muted-foreground">{administrator.account?.email ?? 'Unknown'}</div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{administrator.account_id}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {new Intl.DateTimeFormat(undefined, {
                      year: 'numeric',
                      month: 'short',
                      day: 'numeric',
                    }).format(new Date(administrator.created_at))}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="cursor-pointer text-destructive hover:text-destructive"
                      disabled={removeAdministrator.isPending}
                      onClick={() => {
                        void removeAdministrator.mutateAsync(administrator.account_id).catch(() => {})
                      }}
                    >
                      <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} data-icon="inline-start" />
                      Remove
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!administrators.isLoading && !administrators.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="administrators"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={administrators.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}
    </div>
  )
}
