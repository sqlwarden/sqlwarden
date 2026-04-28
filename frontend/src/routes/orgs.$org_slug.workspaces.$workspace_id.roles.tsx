import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Outlet, createFileRoute, useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { PlusSignIcon, UserShield01Icon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgWorkspaceRolesQueryOptions } from '#/lib/api/query'
import type { Role } from '#/lib/api/types'
import { hasPermission, permission, scopePermissions, type Permission } from '#/lib/permissions'
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
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Skeleton } from '#/components/ui/skeleton'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { Textarea } from '#/components/ui/textarea'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/roles')({
  component: WorkspaceRolesRoute,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function WorkspaceRolesRoute() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const listPath = `/orgs/${orgSlug}/workspaces/${workspaceId}/roles`

  if (trimTrailingSlash(pathname) !== listPath) {
    return <Outlet />
  }

  return <WorkspaceRolesPage orgSlug={orgSlug} workspaceId={workspaceId} />
}

function WorkspaceRolesPage({ orgSlug, workspaceId }: { orgSlug: string; workspaceId: string }) {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [roleName, setRoleName] = useState('')
  const [description, setDescription] = useState('')
  const [selectedPermissions, setSelectedPermissions] = useState<Set<Permission>>(new Set())
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; description?: string; permissions?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const canReadRoles = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canModifyRoles = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)
  const roles = useQuery({
    ...orgWorkspaceRolesQueryOptions(orgSlug, workspaceId, query),
    enabled: canReadRoles,
  })
  const data = roles.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!roles.error) {
      return
    }
    toast.error(roles.error instanceof Error ? roles.error.message : 'Failed to load workspace roles')
  }, [roles.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load role permissions')
  }, [effectivePermissions.error])

  const createRole = useMutation({
    mutationFn: async () =>
      api.post<Role>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/roles`, {
        name: roleName.trim(),
        description: description.trim(),
        permissions: Array.from(selectedPermissions),
      }),
    onSuccess: async () => {
      setIsCreating(false)
      resetCreateRole()
      toast.success('Role created')
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-roles', orgSlug, workspaceId] })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          name: error.fieldErrors?.name,
          description: error.fieldErrors?.description,
          permissions: error.fieldErrors?.permissions,
        })
        if (error.fieldErrors?.name || error.fieldErrors?.description || error.fieldErrors?.permissions) {
          return
        }
      }
      toast.error(error instanceof Error ? error.message : 'Failed to create role')
    },
  })

  const deleteRole = useMutation({
    mutationFn: async (roleId: number) => api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/roles/${roleId}`),
    onSuccess: async () => {
      toast.success('Role deleted')
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-roles', orgSlug, workspaceId] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete role')
    },
  })

  function resetCreateRole() {
    setRoleName('')
    setDescription('')
    setSelectedPermissions(new Set())
    setFieldErrors({})
  }

  function submitCreateRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const errors: typeof fieldErrors = {}
    if (!roleName.trim()) {
      errors.name = 'Name is required'
    }
    if (selectedPermissions.size === 0) {
      errors.permissions = 'Select at least one permission'
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      return
    }
    setFieldErrors({})
    void createRole.mutateAsync().catch(() => {})
  }

  function setPermissionChecked(value: Permission, checked: boolean) {
    setSelectedPermissions((current) => {
      const next = new Set(current)
      if (checked) {
        next.add(value)
      } else {
        next.delete(value)
      }
      return next
    })
    setFieldErrors((current) => ({ ...current, permissions: undefined }))
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">Roles</h1>
            <p className="text-sm text-muted-foreground">
              {!roles.isLoading && total > 0
                ? `${total} workspace role${total !== 1 ? 's' : ''}`
                : 'Workspace-scoped permission sets available for policies.'}
            </p>
          </div>

          {canModifyRoles ? (
            <Dialog
              open={isCreating}
              onOpenChange={(open) => {
                setIsCreating(open)
                if (!open) {
                  resetCreateRole()
                }
              }}
            >
              <DialogTrigger render={<Button />}>
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
                Create
              </DialogTrigger>
              <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
                <DialogHeader>
                  <DialogTitle>Create workspace role</DialogTitle>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-5" onSubmit={submitCreateRole}>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="workspace-role-name">Name</Label>
                    <Input
                      id="workspace-role-name"
                      value={roleName}
                      onChange={(event) => {
                        setRoleName(event.target.value)
                        setFieldErrors((current) => ({ ...current, name: undefined }))
                      }}
                      placeholder="workspace-reader"
                      aria-invalid={fieldErrors.name ? true : undefined}
                      disabled={createRole.isPending}
                    />
                    {fieldErrors.name ? <p className="text-sm text-destructive">{fieldErrors.name}</p> : null}
                  </div>

                  <div className="flex flex-col gap-2">
                    <Label htmlFor="workspace-role-description">Description</Label>
                    <Textarea
                      id="workspace-role-description"
                      value={description}
                      onChange={(event) => {
                        setDescription(event.target.value)
                        setFieldErrors((current) => ({ ...current, description: undefined }))
                      }}
                      placeholder="Describe when this role should be used"
                      aria-invalid={fieldErrors.description ? true : undefined}
                      disabled={createRole.isPending}
                    />
                    {fieldErrors.description ? <p className="text-sm text-destructive">{fieldErrors.description}</p> : null}
                  </div>

                  <PermissionPicker
                    selectedPermissions={selectedPermissions}
                    disabled={createRole.isPending}
                    error={fieldErrors.permissions}
                    onPermissionChecked={setPermissionChecked}
                  />

                  <DialogFooter>
                    <DialogClose render={<Button type="button" variant="ghost" disabled={createRole.isPending} />}>
                      Cancel
                    </DialogClose>
                    <Button type="submit" disabled={createRole.isPending}>
                      {createRole.isPending ? 'Creating...' : 'Create'}
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
          placeholder="Search roles"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Role" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Type" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                {canModifyRoles ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectivePermissions.isLoading || roles.isLoading ? <RolesTableSkeleton canModifyRoles={canModifyRoles} /> : null}
              {roles.isError ? <TableEmptyState colSpan={canModifyRoles ? 4 : 3} icon={UserShield01Icon} message="Failed to load roles." /> : null}
              {!effectivePermissions.isLoading && !canReadRoles ? (
                <TableEmptyState colSpan={canModifyRoles ? 4 : 3} icon={UserShield01Icon} message="You do not have permission to view roles." />
              ) : null}
              {!effectivePermissions.isLoading && canReadRoles && !roles.isLoading && !roles.isError && items.length === 0 ? (
                <TableEmptyState colSpan={canModifyRoles ? 4 : 3} icon={UserShield01Icon} message={query.q ? 'No roles matched your search.' : 'No roles found.'} />
              ) : null}
              {!effectivePermissions.isLoading && canReadRoles && !roles.isLoading && !roles.isError
                ? items.map((role) => (
                    <RoleRow
                      key={role.id}
                      role={role}
                      canModifyRoles={canModifyRoles}
                      isDeleting={deleteRole.isPending}
                      onDelete={(roleId) => deleteRole.mutate(roleId)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {canReadRoles && !roles.isLoading && !roles.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="roles"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={roles.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}
    </div>
  )
}

function RoleRow({
  role,
  canModifyRoles,
  isDeleting,
  onDelete,
}: {
  role: Role
  canModifyRoles: boolean
  isDeleting: boolean
  onDelete: (roleId: number) => void
}) {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const router = useRouter()
  const navigate = useNavigate()

  function preloadRole() {
    void router.preloadRoute({
      to: '/orgs/$org_slug/workspaces/$workspace_id/roles/$role_id',
      params: { org_slug: orgSlug, workspace_id: workspaceId, role_id: String(role.id) },
    })
  }

  function openRole() {
    void navigate({
      to: '/orgs/$org_slug/workspaces/$workspace_id/roles/$role_id',
      params: { org_slug: orgSlug, workspace_id: workspaceId, role_id: String(role.id) },
    })
  }

  return (
    <TableRow
      className="cursor-pointer"
      tabIndex={0}
      role="link"
      onFocus={preloadRole}
      onMouseEnter={preloadRole}
      onClick={openRole}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          openRole()
        }
      }}
    >
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
            {roleDisplayName(role).slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{roleDisplayName(role)}</div>
            <div className="truncate text-muted-foreground">{role.description || 'No description'}</div>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant={role.is_builtin ? 'secondary' : 'outline'}>{role.is_builtin ? 'System' : 'Custom'}</Badge>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(role.created_at)}</TableCell>
      {canModifyRoles ? (
        <TableCell className="text-end">
          {!role.is_builtin ? (
            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button
                    variant="destructive"
                    disabled={isDeleting}
                    onClick={(event) => event.stopPropagation()}
                  />
                }
              >
                Delete
              </AlertDialogTrigger>
              <AlertDialogContent size="sm" onClick={(event) => event.stopPropagation()}>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete role?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This permanently deletes {roleDisplayName(role)}. Any policies using this role will no longer grant its permissions.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel variant="ghost" disabled={isDeleting}>
                    Cancel
                  </AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    disabled={isDeleting}
                    onClick={(event) => {
                      event.stopPropagation()
                      onDelete(role.id)
                    }}
                  >
                    Delete
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function PermissionPicker({
  selectedPermissions,
  disabled,
  error,
  onPermissionChecked,
}: {
  selectedPermissions: Set<Permission>
  disabled: boolean
  error?: string
  onPermissionChecked: (value: Permission, checked: boolean) => void
}) {
  const groupedPermissions = groupPermissions(scopePermissions.workspace)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <Label>Permissions</Label>
        <p className="text-sm text-muted-foreground">Select the capabilities this role should grant in this workspace.</p>
      </div>
      <div className="grid gap-4 rounded-md border border-border p-4 sm:grid-cols-2">
        {groupedPermissions.map((group) => (
          <div key={group.name} className="flex flex-col gap-2">
            <p className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">{group.name}</p>
            <div className="flex flex-col gap-2">
              {group.permissions.map((item) => {
                const id = `workspace-permission-${item.replace(/[^a-z0-9]+/g, '-')}`
                return (
                  <label key={item} htmlFor={id} className="flex cursor-pointer items-center gap-2 text-sm">
                    <Checkbox
                      id={id}
                      checked={selectedPermissions.has(item)}
                      disabled={disabled}
                      onCheckedChange={(checked) => onPermissionChecked(item, checked === true)}
                    />
                    <span>{item}</span>
                  </label>
                )
              })}
            </div>
          </div>
        ))}
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}

function RolesTableSkeleton({ canModifyRoles }: { canModifyRoles: boolean }) {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-md" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-48" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-5 w-16" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          {canModifyRoles ? (
            <TableCell className="text-end">
              <Skeleton className="ms-auto h-8 w-16" />
            </TableCell>
          ) : null}
        </TableRow>
      ))}
    </>
  )
}

function groupPermissions(permissions: readonly Permission[]) {
  const groups = new Map<string, Permission[]>()
  for (const item of permissions) {
    const [prefix] = item.split(':')
    const group = permissionGroupLabel(prefix)
    groups.set(group, [...(groups.get(group) ?? []), item])
  }
  return Array.from(groups.entries()).map(([name, items]) => ({ name, permissions: items }))
}

function permissionGroupLabel(prefix: string) {
  switch (prefix) {
    case 'ws':
      return 'Workspace'
    case 'env':
      return 'Environment'
    case 'conn':
      return 'Connection'
    case 'policy':
      return 'Policy'
    default:
      return prefix.toUpperCase()
  }
}

function roleDisplayName(role: Role) {
  switch (role.name) {
    case 'ws:admin':
      return 'Workspace Admin'
    case 'ws:member':
      return 'Workspace Member'
    default:
      return role.name
  }
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return dateFormatter.format(date)
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
