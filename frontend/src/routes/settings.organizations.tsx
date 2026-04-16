import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { ArrowDown01Icon, ArrowUp01Icon, ArrowUpDownIcon, Cancel01Icon, PlusSignIcon, Search01Icon } from '@hugeicons/core-free-icons'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { instanceOrganizationsQueryOptions, queryKeys } from '#/lib/api/query'
import type { ListQuery, SortOrder } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/settings/organizations')({
  component: SettingsOrganizationsPage,
})

function SettingsOrganizationsPage() {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [newOrganizationName, setNewOrganizationName] = useState('')
  const [newOrganizationSlug, setNewOrganizationSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [createFieldErrors, setCreateFieldErrors] = useState<{ name?: string; slug?: string }>({})
  const [searchText, setSearchText] = useState('')
  const [query, setQuery] = useState<ListQuery>({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'desc',
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

  const organizations = useQuery(instanceOrganizationsQueryOptions(query))
  const data = organizations.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!organizations.error) {
      return
    }

    toast.error(organizations.error instanceof Error ? organizations.error.message : 'Failed to load organizations')
  }, [organizations.error])

  useEffect(() => {
    if (slugTouched) {
      return
    }
    setNewOrganizationSlug(slugify(newOrganizationName))
  }, [newOrganizationName, slugTouched])

  const createOrganization = useMutation({
    mutationFn: async (name: string) =>
      api.post<{ id: number; slug: string; name: string; created_at: string; updated_at: string }>('/api/v1/orgs', {
        name,
        slug: newOrganizationSlug.trim(),
      }),
    onSuccess: async () => {
      setIsCreating(false)
      setNewOrganizationName('')
      setNewOrganizationSlug('')
      setSlugTouched(false)
      setCreateFieldErrors({})
      toast.success('Organization created')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['instance-organizations'] }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
    },
    onError: (error) => {
      if (isApiError(error)) {
        setCreateFieldErrors({
          name: error.fieldErrors?.name,
          slug: error.fieldErrors?.slug,
        })
        if (!error.fieldErrors?.name && !error.fieldErrors?.slug) {
          toast.error(error.message)
        }
        return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to create organization')
    },
  })

  function clearSearch() {
    setSearchText('')
  }

  function submitCreateOrganization(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const name = newOrganizationName.trim()
    const slug = newOrganizationSlug.trim()
    if (!name) {
      setCreateFieldErrors({ name: 'Organization name is required' })
      return
    }

    if (!slug) {
      setCreateFieldErrors({ slug: 'Slug is required' })
      return
    }

    setCreateFieldErrors({})
    void createOrganization.mutateAsync(name).catch(() => {})
  }

  function toggleSort(sort: 'name' | 'slug' | 'created_at') {
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

  function sortIcon(sort: 'name' | 'slug' | 'created_at') {
    if (query.sort !== sort) {
      return <HugeiconsIcon icon={ArrowUpDownIcon} strokeWidth={2} className="size-4" />
    }

    return query.order === 'asc' ? <HugeiconsIcon icon={ArrowUp01Icon} strokeWidth={2} className="size-4" /> : <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} className="size-4" />
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="text-2xl font-semibold tracking-tight">Organizations</h2>
          <Dialog
            open={isCreating}
            onOpenChange={(open) => {
              setIsCreating(open)
              if (!open) {
                setNewOrganizationName('')
                setNewOrganizationSlug('')
                setSlugTouched(false)
                setCreateFieldErrors({})
              }
            }}
          >
            <DialogTrigger render={<Button />}>
              <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
              Create
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create organization</DialogTitle>
                <DialogDescription>Add a new organization to this instance.</DialogDescription>
              </DialogHeader>
              <form className="mt-6 flex flex-col gap-4" onSubmit={submitCreateOrganization}>
                <div className="flex flex-col gap-2">
                  <Input
                    value={newOrganizationName}
                    onChange={(event) => {
                      setNewOrganizationName(event.target.value)
                      setCreateFieldErrors((current) => ({ ...current, name: undefined }))
                    }}
                    placeholder="Organization name"
                    aria-invalid={createFieldErrors.name ? true : undefined}
                    disabled={createOrganization.isPending}
                  />
                  {createFieldErrors.name ? <p className="text-sm text-destructive">{createFieldErrors.name}</p> : null}
                </div>

                <div className="flex flex-col gap-2">
                  <Input
                    value={newOrganizationSlug}
                    onChange={(event) => {
                      setSlugTouched(true)
                      setNewOrganizationSlug(slugify(event.target.value))
                      setCreateFieldErrors((current) => ({ ...current, slug: undefined }))
                    }}
                    placeholder="organization-slug"
                    aria-invalid={createFieldErrors.slug ? true : undefined}
                    disabled={createOrganization.isPending}
                  />
                  {createFieldErrors.slug ? <p className="text-sm text-destructive">{createFieldErrors.slug}</p> : null}
                </div>

                <DialogFooter>
                  <DialogClose render={<Button type="button" variant="ghost" disabled={createOrganization.isPending} />}>
                    Cancel
                  </DialogClose>
                  <Button type="submit" disabled={createOrganization.isPending}>
                    {createOrganization.isPending ? 'Creating…' : 'Create'}
                  </Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>

        <div className="relative max-w-sm">
          <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(event) => setSearchText(event.target.value)}
            placeholder="Search organizations"
            className="pe-9 ps-9"
          />
          {searchText ? (
            <button
              type="button"
              aria-label="Clear search"
              className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
              onClick={clearSearch}
            >
              <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-4" />
            </button>
          ) : null}
        </div>
      </div>

      <Card>
        <CardContent className="flex flex-col gap-4">
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    <button
                      type="button"
                      className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
                      onClick={() => toggleSort('name')}
                    >
                      Name
                      {sortIcon('name')}
                    </button>
                  </TableHead>
                  <TableHead>
                    <button
                      type="button"
                      className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
                      onClick={() => toggleSort('slug')}
                    >
                      Slug
                      {sortIcon('slug')}
                    </button>
                  </TableHead>
                  <TableHead>
                    <button
                      type="button"
                      className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
                      onClick={() => toggleSort('created_at')}
                    >
                      Created
                      {sortIcon('created_at')}
                    </button>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {organizations.isLoading ? (
                  <TableRow>
                    <TableCell colSpan={3} className="py-10 text-center text-sm text-muted-foreground">
                      Loading organizations…
                    </TableCell>
                  </TableRow>
                ) : null}
                {organizations.isError ? (
                  <TableRow>
                    <TableCell colSpan={3} className="py-10 text-center text-sm text-muted-foreground">
                      Failed to load organizations.
                    </TableCell>
                  </TableRow>
                ) : null}
                {!organizations.isLoading && !organizations.isError && items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={3} className="py-10 text-center text-sm text-muted-foreground">
                      {query.q ? 'No organizations matched your search.' : 'No organizations exist yet.'}
                    </TableCell>
                  </TableRow>
                ) : null}
                {items.map((organization) => (
                  <TableRow key={organization.id}>
                    <TableCell className="font-medium text-foreground">{organization.name}</TableCell>
                    <TableCell className="text-muted-foreground">{organization.slug}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {new Intl.DateTimeFormat(undefined, {
                        year: 'numeric',
                        month: 'short',
                        day: 'numeric',
                      }).format(new Date(organization.created_at))}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-muted-foreground">
              {total === 0 ? '0 organizations' : `${(page - 1) * pageSize + 1}-${Math.min(page * pageSize, total)} of ${total} organizations`}
            </p>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                onClick={() => setQuery((current) => ({ ...current, page: Math.max(1, Number(current.page ?? 1) - 1) }))}
                disabled={page <= 1 || organizations.isFetching}
              >
                Previous
              </Button>
              <div className="min-w-20 text-center text-sm text-muted-foreground">
                Page {page} of {pageCount}
              </div>
              <Button
                variant="outline"
                onClick={() => setQuery((current) => ({ ...current, page: Number(current.page ?? 1) + 1 }))}
                disabled={page >= pageCount || organizations.isFetching}
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

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
}
