import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon } from '@hugeicons/core-free-icons'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { instanceOrganizationsQueryOptions, queryKeys } from '#/lib/api/query'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { getInitials } from '#/components/InitialsAvatar'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/settings/organizations')({
  component: SettingsOrganizationsPage,
  pendingComponent: RoutePending,
})

function SettingsOrganizationsPage() {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [newOrganizationName, setNewOrganizationName] = useState('')
  const [newOrganizationSlug, setNewOrganizationSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [createFieldErrors, setCreateFieldErrors] = useState<{ name?: string; slug?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'desc',
    q: '',
  })

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

  function submitCreateOrganization(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const name = newOrganizationName.trim()
    const slug = newOrganizationSlug.trim()
    if (!name) {
      setCreateFieldErrors({ name: 'Organization name is required.' })
      return
    }

    if (!slug) {
      setCreateFieldErrors({ slug: 'Slug is required.' })
      return
    }

    setCreateFieldErrors({})
    void createOrganization.mutateAsync(name).catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h2 className="text-2xl font-semibold tracking-tight">Organizations</h2>
            <p className="text-sm text-muted-foreground">
              {!organizations.isLoading && total > 0
                ? `${total} organization${total !== 1 ? 's' : ''} on this instance`
                : 'All organizations on this instance.'}
            </p>
          </div>
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

        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search organizations"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Organization" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {organizations.isLoading ? (
                <TableEmptyState colSpan={2} compact message="Loading organizations…" />
              ) : null}
              {organizations.isError ? (
                <TableEmptyState colSpan={2} compact message="Failed to load organizations." />
              ) : null}
              {!organizations.isLoading && !organizations.isError && items.length === 0 ? (
                <TableEmptyState colSpan={2} compact message={query.q ? 'No organizations matched your search.' : 'No organizations exist yet.'} />
              ) : null}
              {items.map((organization) => (
                <TableRow key={organization.id}>
                  <TableCell>
                    <div className="flex min-w-0 items-center gap-3">
                      <div className={cn('flex size-8 shrink-0 items-center justify-center text-xs font-semibold', orgColor(organization.name))}>
                        {getInitials(organization.name, 'O')}
                      </div>
                      <div className="min-w-0">
                        <div className="truncate font-medium text-foreground">{organization.name}</div>
                        <div className="truncate text-muted-foreground">@{organization.slug}</div>
                      </div>
                    </div>
                  </TableCell>
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
        </CardContent>
      </Card>

      {!organizations.isLoading && !organizations.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="organizations"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={organizations.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}
    </div>
  )
}

const ORG_COLORS = [
  'bg-orange-500/10 text-orange-600',
  'bg-blue-500/10 text-blue-600',
  'bg-emerald-500/10 text-emerald-600',
  'bg-violet-500/10 text-violet-600',
  'bg-rose-500/10 text-rose-600',
  'bg-amber-500/10 text-amber-600',
  'bg-cyan-500/10 text-cyan-600',
]

function orgColor(name: string): string {
  const hash = name.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
  return ORG_COLORS[hash % ORG_COLORS.length]
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
}
