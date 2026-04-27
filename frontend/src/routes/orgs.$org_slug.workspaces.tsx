import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Briefcase01Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgWorkspacesQueryOptions } from '#/lib/api/query'
import type { Workspace } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { EmptyState } from '#/components/EmptyState'
import { getInitials } from '#/components/InitialsAvatar'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Separator } from '#/components/ui/separator'
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
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [newWorkspaceName, setNewWorkspaceName] = useState('')
  const [newWorkspaceDescription, setNewWorkspaceDescription] = useState('')
  const [createFieldErrors, setCreateFieldErrors] = useState<{ name?: string; description?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize } = useListPageState({
    page: 1,
    page_size: 12,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const workspaces = useQuery(orgWorkspacesQueryOptions(orgSlug, query))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canCreateWorkspace = hasPermission(effectivePermissions.data?.permissions, permission.wsCreate)
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

    toast.error(workspaces.error instanceof Error ? workspaces.error.message : 'Failed to load workspaces')
  }, [workspaces.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }

    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load workspace permissions')
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
      await queryClient.invalidateQueries({ queryKey: ['org-workspaces', orgSlug] })
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
      toast.error(error instanceof Error ? error.message : 'Failed to create workspace')
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
      setCreateFieldErrors({ name: 'Workspace name is required' })
      return
    }

    setCreateFieldErrors({})
    void createWorkspace.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">Workspaces</h1>
            <p className="text-sm text-muted-foreground">Choose a workspace to continue.</p>
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
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
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
                    {createFieldErrors.name ? <p className="text-sm text-destructive">{createFieldErrors.name}</p> : null}
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
                    {createFieldErrors.description ? <p className="text-sm text-destructive">{createFieldErrors.description}</p> : null}
                  </div>

                  <DialogFooter>
                    <DialogClose render={<Button type="button" variant="ghost" disabled={createWorkspace.isPending} />}>
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
            <Card key={index}>
              <CardContent className="flex min-h-48 flex-col justify-between gap-6">
                <div className="flex items-start gap-4">
                  <Skeleton className="size-12 shrink-0 rounded-lg" />
                  <div className="flex flex-1 flex-col gap-2">
                    <Skeleton className="h-5 w-32" />
                    <Skeleton className="h-4 w-48" />
                  </div>
                </div>
                <Skeleton className="h-9 w-20 self-end" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}

      {workspaces.isError ? (
        <Card>
          <CardContent>
            <EmptyState icon={Briefcase01Icon} message="Failed to load workspaces" description="Refresh the page and try again." />
          </CardContent>
        </Card>
      ) : null}

      {!workspaces.isLoading && !workspaces.isError && items.length === 0 ? (
        <Card>
          <CardContent>
            <EmptyState
              icon={Briefcase01Icon}
              message={query.q ? 'No workspaces matched your search.' : 'No workspaces found'}
              description={query.q ? 'Try a different workspace name.' : 'This organization does not have any visible workspaces yet.'}
            />
          </CardContent>
        </Card>
      ) : null}

      {!workspaces.isLoading && !workspaces.isError && items.length > 0 ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {items.map((workspace) => (
              <Card
                key={workspace.id}
                className="h-full gap-4 py-4 transition-colors hover:border-foreground/20 hover:bg-muted/30"
              >
                <CardHeader className="flex flex-row items-start gap-4">
                  <div className="flex size-12 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-semibold text-foreground">
                    {getInitials(workspace.name, 'W')}
                  </div>
                  <div className="min-w-0 flex-1">
                    <CardTitle className="truncate text-base">{workspace.name}</CardTitle>
                    <CardDescription className="line-clamp-2">
                      {workspace.description || 'No description provided.'}
                    </CardDescription>
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="flex flex-col gap-4">
                    <Separator />
                    <div className="grid grid-cols-2 gap-4">
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted-foreground uppercase">Environments</span>
                        <span className="text-lg font-semibold text-foreground">{workspace.environment_count}</span>
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted-foreground uppercase">Connections</span>
                        <span className="text-lg font-semibold text-foreground">{workspace.connection_count}</span>
                      </div>
                    </div>
                    <Separator />
                  </div>
                </CardContent>
                <CardFooter className="justify-end">
                  <Button
                    className="w-auto px-4"
                    nativeButton={false}
                    render={
                      <Link
                        to="/orgs/$org_slug/workspaces/$workspace_id"
                        params={{ org_slug: orgSlug, workspace_id: String(workspace.id) }}
                      />
                    }
                  >
                    Open
                  </Button>
                </CardFooter>
              </Card>
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

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
