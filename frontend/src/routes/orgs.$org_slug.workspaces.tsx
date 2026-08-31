import { errorMessage } from '#/lib/api/errors'
import { useOrganizationPageTitle } from '#/lib/page-title'
import { trimTrailingSlash } from '#/lib/utils'
import { useEffect, useState } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgWorkspacesQueryOptions } from '#/lib/api/query'
import type { Workspace } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
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
import { Input } from '#/components/ui/input'
import { EmptyState } from '#/components/EmptyState'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Skeleton } from '#/components/ui/skeleton'

export const Route = createFileRoute('/orgs/$org_slug/workspaces')({
  component: OrganizationWorkspacesRoute,
  pendingComponent: RoutePending,
})

function OrganizationWorkspacesRoute() {
  const { org_slug: orgSlug } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const listPath = `/orgs/${orgSlug}/workspaces`

  if (trimTrailingSlash(pathname) !== listPath) {
    return <Outlet />
  }

  return <OrganizationWorkspacesPage orgSlug={orgSlug} />
}

function OrganizationWorkspacesPage({ orgSlug }: { orgSlug: string }) {
  useOrganizationPageTitle('Workspaces')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [newWorkspaceName, setNewWorkspaceName] = useState('')
  const [newWorkspaceDescription, setNewWorkspaceDescription] = useState('')
  const [createFieldErrors, setCreateFieldErrors] = useState<{
    name?: string
    description?: string
  }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize } = useListPageState({
    page: 1,
    page_size: 12,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const workspaces = useQuery(orgWorkspacesQueryOptions(orgSlug, query))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canCreateWorkspace = hasPermission(
    effectivePermissions.data?.permissions,
    permission.wsCreate,
  )
  const data = workspaces.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 12)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!workspaces.error) {
      return
    }

    toast.error(errorMessage(workspaces.error, 'Failed to load workspaces'))
  }, [workspaces.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }

    toast.error(errorMessage(effectivePermissions.error, 'Failed to load workspace permissions'))
  }, [effectivePermissions.error])

  const createWorkspace = useMutation({
    mutationFn: async () =>
      api.post<Workspace>(`/api/v1/orgs/${orgSlug}/workspaces`, {
        name: newWorkspaceName.trim(),
        description: newWorkspaceDescription.trim(),
      }),
    onSuccess: async (workspace) => {
      setIsCreating(false)
      setNewWorkspaceName('')
      setNewWorkspaceDescription('')
      setCreateFieldErrors({})
      toast.success('Workspace created')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspacesScope(orgSlug) })
      await navigate({
        to: '/orgs/$org_slug/workspaces/$workspace_id',
        params: { org_slug: orgSlug, workspace_id: String(workspace.id) },
      })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setCreateFieldErrors({
          name: error.fieldErrors?.name,
          description: error.fieldErrors?.description,
        })
        if (!error.fieldErrors?.name && !error.fieldErrors?.description) {
          toast.error(error.message)
        }
        return
      }
      toast.error(errorMessage(error, 'Failed to create workspace'))
    },
  })

  function resetCreateWorkspace() {
    setNewWorkspaceName('')
    setNewWorkspaceDescription('')
    setCreateFieldErrors({})
  }

  function submitCreateWorkspace(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!newWorkspaceName.trim()) {
      setCreateFieldErrors({ name: 'Workspace name is required.' })
      return
    }

    setCreateFieldErrors({})
    void createWorkspace.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="font-heading text-2xl font-semibold tracking-tight">Workspaces</h1>
            <p className="text-sm text-muted-foreground">
              {!workspaces.isLoading && total > 0
                ? `${total} workspace${total !== 1 ? 's' : ''} in @${orgSlug}`
                : 'Choose a workspace to continue.'}
            </p>
          </div>

          {canCreateWorkspace ? (
            <Dialog
              open={isCreating}
              onOpenChange={(open) => {
                setIsCreating(open)
                if (!open) {
                  resetCreateWorkspace()
                }
              }}
            >
              <DialogTrigger render={<Button />}>
                <Icon name="plus-sign" size={20} data-icon="inline-start" />
                Create
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create workspace</DialogTitle>
                  <DialogDescription>Add a workspace to this organization.</DialogDescription>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-4" onSubmit={submitCreateWorkspace}>
                  <div className="flex flex-col gap-2">
                    <Input
                      value={newWorkspaceName}
                      onChange={(event) => {
                        setNewWorkspaceName(event.target.value)
                        setCreateFieldErrors((current) => ({ ...current, name: undefined }))
                      }}
                      placeholder="Workspace name"
                      aria-invalid={createFieldErrors.name ? true : undefined}
                      disabled={createWorkspace.isPending}
                    />
                    {createFieldErrors.name ? (
                      <p className="text-sm text-destructive">{createFieldErrors.name}</p>
                    ) : null}
                  </div>

                  <div className="flex flex-col gap-2">
                    <Input
                      value={newWorkspaceDescription}
                      onChange={(event) => {
                        setNewWorkspaceDescription(event.target.value)
                        setCreateFieldErrors((current) => ({ ...current, description: undefined }))
                      }}
                      placeholder="Description optional"
                      aria-invalid={createFieldErrors.description ? true : undefined}
                      disabled={createWorkspace.isPending}
                    />
                    {createFieldErrors.description ? (
                      <p className="text-sm text-destructive">{createFieldErrors.description}</p>
                    ) : null}
                  </div>

                  <DialogFooter>
                    <DialogClose
                      render={
                        <Button
                          type="button"
                          variant="ghost"
                          disabled={createWorkspace.isPending}
                        />
                      }
                    >
                      Cancel
                    </DialogClose>
                    <Button type="submit" disabled={createWorkspace.isPending}>
                      {createWorkspace.isPending ? 'Creating...' : 'Create'}
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
          placeholder="Search workspaces"
        />
      </div>

      {workspaces.isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="flex flex-col rounded-lg border border-border bg-card">
              <div className="flex flex-col gap-3 p-5">
                <div className="flex items-start gap-3">
                  <Skeleton className="size-9 shrink-0 rounded-md" />
                  <div className="flex flex-1 flex-col gap-2 pt-1">
                    <Skeleton className="h-4 w-28" />
                    <Skeleton className="h-3 w-44" />
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-4 border-t border-border/60 px-5 py-3">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-3 w-20" />
              </div>
            </div>
          ))}
        </div>
      ) : null}

      {workspaces.isError ? (
        <Card>
          <CardContent>
            <EmptyState
              icon="briefcase-01"
              message="Failed to load workspaces"
              description="Refresh the page and try again."
            />
          </CardContent>
        </Card>
      ) : null}

      {!workspaces.isLoading && !workspaces.isError && items.length === 0 ? (
        <Card>
          <CardContent>
            <EmptyState
              icon="briefcase-01"
              message={query.q ? 'No workspaces matched your search.' : 'No workspaces found'}
              description={
                query.q
                  ? 'Try a different workspace name.'
                  : 'This organization does not have any visible workspaces yet.'
              }
            />
          </CardContent>
        </Card>
      ) : null}

      {!workspaces.isLoading && !workspaces.isError && items.length > 0 ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {items.map((workspace) => (
              <Link
                key={workspace.id}
                to="/orgs/$org_slug/workspaces/$workspace_id"
                params={{ org_slug: orgSlug, workspace_id: String(workspace.id) }}
                className="group relative flex flex-col overflow-hidden rounded-lg border border-border bg-card text-card-foreground transition-all hover:border-foreground/20 hover:shadow-sm"
              >
                <span className="absolute inset-y-0 left-0 w-0.5 scale-y-0 bg-primary transition-transform group-hover:scale-y-100" />
                <div className="flex flex-1 items-start gap-3 p-5">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground transition-colors group-hover:text-primary">
                    <Icon name="briefcase-01" size={18} />
                  </div>
                  <div className="min-w-0 flex-1 pt-0.5">
                    <p className="truncate font-semibold leading-tight tracking-tight transition-colors group-hover:text-primary">
                      {workspace.name}
                    </p>
                    {workspace.description ? (
                      <p className="mt-1.5 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
                        {workspace.description}
                      </p>
                    ) : null}
                  </div>
                </div>
                <div className="flex items-center gap-4 border-t border-border/60 px-5 py-3 text-xs text-muted-foreground">
                  <div className="flex items-center gap-1.5">
                    <Icon name="database" size={14} />
                    <span className="tabular-nums">{workspace.environment_count}</span>
                    <span>
                      {workspace.environment_count === 1 ? 'environment' : 'environments'}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Icon name="flow-connection" size={14} />
                    <span className="tabular-nums">{workspace.connection_count}</span>
                    <span>{workspace.connection_count === 1 ? 'connection' : 'connections'}</span>
                  </div>
                </div>
              </Link>
            ))}
          </div>

          <PaginationFooter
            itemLabel="workspaces"
            page={page}
            pageCount={pageCount}
            pageSize={pageSize}
            total={total}
            isFetching={workspaces.isFetching}
            pageSizeOptions={[12, 24, 48, 96]}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        </>
      ) : null}
    </div>
  )
}
