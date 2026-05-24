import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { DatabaseIcon, Delete02Icon, PencilEdit02Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgEnvironmentsQueryOptions } from '#/lib/api/query'
import type { Environment } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { useListPageState } from '#/hooks/use-list-page-state'
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
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Textarea } from '#/components/ui/textarea'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Skeleton } from '#/components/ui/skeleton'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/environments')({
  component: WorkspaceEnvironmentsPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

type EnvironmentFormValues = {
  name: string
  description: string
}

type EnvironmentFieldErrors = {
  name?: string
  description?: string
}

function WorkspaceEnvironmentsPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editingEnvironment, setEditingEnvironment] = useState<Environment | null>(null)
  const [createValues, setCreateValues] = useState<EnvironmentFormValues>({ name: '', description: '' })
  const [editValues, setEditValues] = useState<EnvironmentFormValues>({ name: '', description: '' })
  const [createErrors, setCreateErrors] = useState<EnvironmentFieldErrors>({})
  const [editErrors, setEditErrors] = useState<EnvironmentFieldErrors>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'asc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const canCreateEnvironment = hasPermission(effectivePermissions.data?.permissions, permission.envCreate)
  const canEditEnvironment = hasPermission(effectivePermissions.data?.permissions, permission.envWrite)
  const canDeleteEnvironment = hasPermission(effectivePermissions.data?.permissions, permission.envDelete)
  const canManageEnvironment = canEditEnvironment || canDeleteEnvironment
  const environments = useQuery(orgEnvironmentsQueryOptions(orgSlug, workspaceId, query))

  const items = environments.data?.items ?? []
  const page = environments.data?.page ?? Number(query.page ?? 1)
  const pageSize = environments.data?.page_size ?? Number(query.page_size ?? 10)
  const total = environments.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1
  const tableColumnCount = canManageEnvironment ? 4 : 3

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load environment permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!environments.error) return
    toast.error(environments.error instanceof Error ? environments.error.message : 'Failed to load environments')
  }, [environments.error])

  const createEnvironment = useMutation({
    mutationFn: async () =>
      api.post<Environment>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/environments`, {
        name: createValues.name.trim(),
        description: createValues.description.trim(),
      }),
    onSuccess: async () => {
      setCreateOpen(false)
      setCreateValues({ name: '', description: '' })
      setCreateErrors({})
      toast.success('Environment created')
      await queryClient.invalidateQueries({ queryKey: ['org-environments', orgSlug, workspaceId] })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setCreateErrors({
          name: error.fieldErrors.name,
          description: error.fieldErrors.description,
        })
        if (error.fieldErrors.name || error.fieldErrors.description) return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to create environment')
    },
  })

  const updateEnvironment = useMutation({
    mutationFn: async () => {
      if (!editingEnvironment) return
      return api.patch<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/environments/${editingEnvironment.id}`, {
        name: editValues.name.trim(),
        description: editValues.description.trim(),
      })
    },
    onSuccess: async () => {
      setEditingEnvironment(null)
      setEditValues({ name: '', description: '' })
      setEditErrors({})
      toast.success('Environment updated')
      await queryClient.invalidateQueries({ queryKey: ['org-environments', orgSlug, workspaceId] })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setEditErrors({
          name: error.fieldErrors.name,
          description: error.fieldErrors.description,
        })
        if (error.fieldErrors.name || error.fieldErrors.description) return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to update environment')
    },
  })

  const deleteEnvironment = useMutation({
    mutationFn: async (environmentId: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/environments/${environmentId}`),
    onSuccess: async () => {
      toast.success('Environment deleted')
      await queryClient.invalidateQueries({ queryKey: ['org-environments', orgSlug, workspaceId] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete environment')
    },
  })

  function submitCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!createValues.name.trim()) {
      setCreateErrors({ name: 'Name is required.' })
      return
    }
    setCreateErrors({})
    void createEnvironment.mutateAsync().catch(() => {})
  }

  function openEdit(environment: Environment) {
    setEditingEnvironment(environment)
    setEditValues({
      name: environment.name,
      description: environment.description ?? '',
    })
    setEditErrors({})
  }

  function submitEdit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!editValues.name.trim()) {
      setEditErrors({ name: 'Name is required.' })
      return
    }
    setEditErrors({})
    void updateEnvironment.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">Environments</h1>
            <p className="text-sm text-muted-foreground">
              {!environments.isLoading && total > 0
                ? `${total} environment${total !== 1 ? 's' : ''} in this workspace`
                : 'Group connections by deployment environment.'}
            </p>
          </div>

          {canCreateEnvironment ? (
            <Dialog
              open={createOpen}
              onOpenChange={(open) => {
                setCreateOpen(open)
                if (!open) {
                  setCreateValues({ name: '', description: '' })
                  setCreateErrors({})
                }
              }}
            >
              <DialogTrigger render={<Button />}>
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
                Create
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create Environment</DialogTitle>
                </DialogHeader>
                <EnvironmentForm
                  values={createValues}
                  errors={createErrors}
                  isPending={createEnvironment.isPending}
                  submitLabel={createEnvironment.isPending ? 'Creating...' : 'Create'}
                  onValuesChange={setCreateValues}
                  onErrorsChange={setCreateErrors}
                  onSubmit={submitCreate}
                />
              </DialogContent>
            </Dialog>
          ) : null}
        </div>

        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search environments"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Environment" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Description" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                {canManageEnvironment ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {environments.isLoading ? <EnvironmentTableSkeleton canManageEnvironment={canManageEnvironment} /> : null}
              {environments.isError ? <TableEmptyState colSpan={tableColumnCount} icon={DatabaseIcon} message="Failed to load environments." /> : null}
              {!environments.isLoading && !environments.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={tableColumnCount}
                  icon={DatabaseIcon}
                  message={query.q ? 'No environments matched your search.' : 'No environments found.'}
                />
              ) : null}
              {!environments.isLoading && !environments.isError
                ? items.map((environment) => (
                    <EnvironmentRow
                      key={environment.id}
                      environment={environment}
                      canEditEnvironment={canEditEnvironment}
                      canDeleteEnvironment={canDeleteEnvironment}
                      isDeleting={deleteEnvironment.isPending}
                      onEdit={openEdit}
                      onDelete={(environmentId) => deleteEnvironment.mutate(environmentId)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!environments.isLoading && !environments.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="environments"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={environments.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}

      <Dialog
        open={editingEnvironment !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditingEnvironment(null)
            setEditValues({ name: '', description: '' })
            setEditErrors({})
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Environment</DialogTitle>
          </DialogHeader>
          <EnvironmentForm
            values={editValues}
            errors={editErrors}
            isPending={updateEnvironment.isPending}
            submitLabel={updateEnvironment.isPending ? 'Saving...' : 'Save changes'}
            onValuesChange={setEditValues}
            onErrorsChange={setEditErrors}
            onSubmit={submitEdit}
          />
        </DialogContent>
      </Dialog>
    </div>
  )
}

function EnvironmentForm({
  errors,
  isPending,
  onErrorsChange,
  onSubmit,
  onValuesChange,
  submitLabel,
  values,
}: {
  errors: EnvironmentFieldErrors
  isPending: boolean
  onErrorsChange: React.Dispatch<React.SetStateAction<EnvironmentFieldErrors>>
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void
  onValuesChange: React.Dispatch<React.SetStateAction<EnvironmentFormValues>>
  submitLabel: string
  values: EnvironmentFormValues
}) {
  return (
    <form className="mt-6 flex flex-col gap-4" onSubmit={onSubmit}>
      <Field label="Name" error={errors.name}>
        <Input
          value={values.name}
          disabled={isPending}
          aria-invalid={errors.name ? true : undefined}
          onChange={(event) => {
            onValuesChange((current) => ({ ...current, name: event.target.value }))
            onErrorsChange((current) => ({ ...current, name: undefined }))
          }}
        />
      </Field>
      <Field label="Description" error={errors.description}>
        <Textarea
          value={values.description}
          disabled={isPending}
          placeholder="Optional environment description"
          aria-invalid={errors.description ? true : undefined}
          onChange={(event) => {
            onValuesChange((current) => ({ ...current, description: event.target.value }))
            onErrorsChange((current) => ({ ...current, description: undefined }))
          }}
        />
      </Field>
      <DialogFooter>
        <DialogClose render={<Button type="button" variant="ghost" disabled={isPending} />}>Cancel</DialogClose>
        <Button type="submit" disabled={isPending}>
          {submitLabel}
        </Button>
      </DialogFooter>
    </form>
  )
}

function EnvironmentRow({
  canDeleteEnvironment,
  canEditEnvironment,
  environment,
  isDeleting,
  onDelete,
  onEdit,
}: {
  canDeleteEnvironment: boolean
  canEditEnvironment: boolean
  environment: Environment
  isDeleting: boolean
  onDelete: (environmentId: number) => void
  onEdit: (environment: Environment) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md border border-border text-muted-foreground">
            <HugeiconsIcon icon={DatabaseIcon} strokeWidth={2} className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{environment.name}</div>
            <div className="truncate text-muted-foreground">Environment</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="max-w-sm truncate text-muted-foreground">
        {environment.description || <span className="text-muted-foreground">-</span>}
      </TableCell>
      <TableCell className="text-muted-foreground">{dateFormatter.format(new Date(environment.created_at))}</TableCell>
      {canEditEnvironment || canDeleteEnvironment ? (
        <TableCell>
          <div className="flex justify-end gap-2">
            {canEditEnvironment ? (
              <Button type="button" variant="ghost" size="sm" onClick={() => onEdit(environment)}>
                <HugeiconsIcon icon={PencilEdit02Icon} strokeWidth={2} data-icon="inline-start" />
                Edit
              </Button>
            ) : null}
            {canDeleteEnvironment ? (
              <AlertDialog>
                <AlertDialogTrigger render={<Button type="button" variant="destructive" size="sm" disabled={isDeleting} />}>
                  <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} data-icon="inline-start" />
                  Delete
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Delete environment?</AlertDialogTitle>
                    <AlertDialogDescription>
                      This permanently deletes <span className="font-medium text-foreground">{environment.name}</span>. Environments with connections cannot be deleted.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel variant="ghost" disabled={isDeleting}>
                      Cancel
                    </AlertDialogCancel>
                    <AlertDialogAction
                      variant="destructive"
                      disabled={isDeleting}
                      onClick={() => onDelete(environment.id)}
                    >
                      {isDeleting ? 'Deleting...' : 'Delete'}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            ) : null}
          </div>
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function EnvironmentTableSkeleton({ canManageEnvironment }: { canManageEnvironment: boolean }) {
  return Array.from({ length: 4 }).map((_, index) => (
    <TableRow key={index}>
      <TableCell>
        <div className="flex items-center gap-3">
          <Skeleton className="size-8 rounded-md" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-20" />
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-48" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-24" />
      </TableCell>
      {canManageEnvironment ? (
        <TableCell>
          <div className="flex justify-end gap-2">
            <Skeleton className="h-8 w-16" />
            <Skeleton className="h-8 w-20" />
          </div>
        </TableCell>
      ) : null}
    </TableRow>
  ))
}

function Field({
  children,
  error,
  label,
}: {
  children: React.ReactNode
  error?: string
  label: string
}) {
  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
