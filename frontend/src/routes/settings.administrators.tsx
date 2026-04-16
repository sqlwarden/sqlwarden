import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, ArrowUpDown, Plus, Search, Trash2, X } from 'lucide-react'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { instanceAdminsQueryOptions, queryKeys } from '#/lib/api/query'
import type { ListQuery, SortOrder } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/settings/administrators')({
  component: SettingsAdministratorsPage,
})

function SettingsAdministratorsPage() {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [email, setEmail] = useState('')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [searchText, setSearchText] = useState('')
  const [query, setQuery] = useState<ListQuery>({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'asc',
    q: '',
  })

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQuery((current) => {
        const nextSearch = searchText.trim()
        if ((current.q ?? '') === nextSearch) {
          return current
        }

        return {
          ...current,
          page: 1,
          q: nextSearch,
        }
      })
    }, 300)

    return () => window.clearTimeout(timer)
  }, [searchText])

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

  function toggleSort(sort: 'account_id' | 'created_at') {
    setQuery((current) => {
      const currentSort = current.sort
      const currentOrder = (current.order as SortOrder | undefined) ?? 'asc'

      return {
        ...current,
        page: 1,
        sort,
        order: currentSort === sort && currentOrder === 'asc' ? 'desc' : 'asc',
      }
    })
  }

  function sortIcon(sort: 'account_id' | 'created_at') {
    if (query.sort !== sort) {
      return <ArrowUpDown className="size-4" />
    }

    return query.order === 'asc' ? <ArrowUp className="size-4" /> : <ArrowDown className="size-4" />
  }

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

  function clearSearch() {
    setSearchText('')
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-2xl font-semibold tracking-tight">Administrators</h2>
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
            <Plus data-icon="inline-start" />
            Create
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

      <div className="relative max-w-sm">
        <Search className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={searchText}
          onChange={(event) => setSearchText(event.target.value)}
          placeholder="Search administrators"
          className="pe-9 ps-9"
        />
        {searchText ? (
          <button
            type="button"
            aria-label="Clear search"
            className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
            onClick={clearSearch}
          >
            <X className="size-4" />
          </button>
        ) : null}
      </div>

      <Card>
        <CardContent className="flex flex-col gap-4">
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>
                    <button
                      type="button"
                      className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
                      onClick={() => toggleSort('account_id')}
                    >
                      Account ID
                      {sortIcon('account_id')}
                    </button>
                  </TableHead>
                  <TableHead>
                    <button
                      type="button"
                      className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
                      onClick={() => toggleSort('created_at')}
                    >
                      Added
                      {sortIcon('created_at')}
                    </button>
                  </TableHead>
                  <TableHead className="w-1 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {administrators.isLoading ? (
                  <TableRow>
                    <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                      Loading administrators…
                    </TableCell>
                  </TableRow>
                ) : null}
                {administrators.isError ? (
                  <TableRow>
                    <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                      Failed to load administrators.
                    </TableCell>
                  </TableRow>
                ) : null}
                {!administrators.isLoading && !administrators.isError && items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                      {query.q ? 'No administrators matched your search.' : 'No administrators exist yet.'}
                    </TableCell>
                  </TableRow>
                ) : null}
                {items.map((administrator) => (
                  <TableRow key={administrator.account_id}>
                    <TableCell className="font-medium text-foreground">{administrator.account?.name ?? 'Unknown'}</TableCell>
                    <TableCell className="text-muted-foreground">{administrator.account?.email ?? 'Unknown'}</TableCell>
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
                        <Trash2 data-icon="inline-start" />
                        Remove
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-muted-foreground">
              {total === 0 ? '0 administrators' : `${(page - 1) * pageSize + 1}-${Math.min(page * pageSize, total)} of ${total} administrators`}
            </p>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                onClick={() => setQuery((current) => ({ ...current, page: Math.max(1, Number(current.page ?? 1) - 1) }))}
                disabled={page <= 1 || administrators.isFetching}
              >
                Previous
              </Button>
              <div className="min-w-20 text-center text-sm text-muted-foreground">
                Page {page} of {pageCount}
              </div>
              <Button
                variant="outline"
                onClick={() => setQuery((current) => ({ ...current, page: Number(current.page ?? 1) + 1 }))}
                disabled={page >= pageCount || administrators.isFetching}
              >
                Next
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
