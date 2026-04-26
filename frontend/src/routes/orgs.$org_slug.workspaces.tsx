import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Briefcase01Icon, Cancel01Icon, PlusSignIcon, Search01Icon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgWorkspacesQueryOptions } from '#/lib/api/query'
import type { ListQuery, Workspace } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '#/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Separator } from '#/components/ui/separator'
import { Skeleton } from '#/components/ui/skeleton'

export const Route = createFileRoute('/orgs/$org_slug/workspaces')({
  component: OrganizationWorkspacesRoute,
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
  const [searchText, setSearchText] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const [newWorkspaceName, setNewWorkspaceName] = useState('')
  const [newWorkspaceDescription, setNewWorkspaceDescription] = useState('')
  const [createFieldErrors, setCreateFieldErrors] = useState<{ name?: string; description?: string }>({})
  const [query, setQuery] = useState<ListQuery>({
    page: 1,
    page_size: 12,
    sort: 'name',
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

  function clearSearch() {
    setSearchText('')
  }

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

        <div className="relative max-w-md">
          <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(event) => setSearchText(event.target.value)}
            placeholder="Search workspaces"
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
          <CardContent className="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
            <HugeiconsIcon icon={Briefcase01Icon} strokeWidth={2} className="size-10 text-muted-foreground" />
            <div className="flex flex-col gap-1">
              <p className="font-medium text-foreground">Failed to load workspaces</p>
              <p className="text-sm text-muted-foreground">Refresh the page and try again.</p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {!workspaces.isLoading && !workspaces.isError && items.length === 0 ? (
        <Card>
          <CardContent className="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
            <HugeiconsIcon icon={Briefcase01Icon} strokeWidth={2} className="size-10 text-muted-foreground" />
            <div className="flex flex-col gap-1">
              <p className="font-medium text-foreground">
                {query.q ? 'No workspaces matched your search.' : 'No workspaces found'}
              </p>
              <p className="text-sm text-muted-foreground">
                {query.q ? 'Try a different workspace name.' : 'This organization does not have any visible workspaces yet.'}
              </p>
            </div>
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
                    {workspaceInitials(workspace.name)}
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

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-muted-foreground">
              {total === 0 ? '0 workspaces' : `${(page - 1) * pageSize + 1}-${Math.min(page * pageSize, total)} of ${total} workspaces`}
            </p>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                onClick={() => setQuery((current) => ({ ...current, page: Math.max(1, Number(current.page ?? 1) - 1) }))}
                disabled={page <= 1 || workspaces.isFetching}
              >
                Previous
              </Button>
              <div className="min-w-20 text-center text-sm text-muted-foreground">
                Page {page} of {pageCount}
              </div>
              <Button
                variant="outline"
                onClick={() => setQuery((current) => ({ ...current, page: Number(current.page ?? 1) + 1 }))}
                disabled={page >= pageCount || workspaces.isFetching}
              >
                Next
              </Button>
            </div>
          </div>
        </>
      ) : null}
    </div>
  )
}

function workspaceInitials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return 'W'
  }

  return parts
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
