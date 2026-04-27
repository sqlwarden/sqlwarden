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
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

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

        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search organizations"
          className="max-w-sm"
        />
      </div>

      <Card>
        <CardContent className="flex flex-col gap-4">
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    <TableColumnHeader label="Name" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                  </TableHead>
                  <TableHead>
                    <TableColumnHeader label="Slug" sort="slug" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                  </TableHead>
                  <TableHead>
                    <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {organizations.isLoading ? (
                  <TableEmptyState colSpan={3} compact message="Loading organizations…" />
                ) : null}
                {organizations.isError ? (
                  <TableEmptyState colSpan={3} compact message="Failed to load organizations." />
                ) : null}
                {!organizations.isLoading && !organizations.isError && items.length === 0 ? (
                  <TableEmptyState colSpan={3} compact message={query.q ? 'No organizations matched your search.' : 'No organizations exist yet.'} />
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
