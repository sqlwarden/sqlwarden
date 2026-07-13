import { errorMessage } from '#/lib/api/errors'
import { trimTrailingSlash } from '#/lib/utils'
import { formatDate } from '#/lib/format'
import { useEffect, useState, type FormEvent } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Outlet, createFileRoute, useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgPermissionsQueryOptions, orgWorkspaceRolesQueryOptions } from '#/lib/api/query'
import type { Role, RoleScope } from '#/lib/api/types'
import { hasPermission, permission, permissionDefinitionMap, type Permission } from '#/lib/permissions'
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
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '#/components/ui/tooltip'
import { SearchInput } from '#/components/SearchInput'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { Textarea } from '#/components/ui/textarea'
import { PermissionPicker } from '#/components/access-control/PermissionPicker'
import { RolesTableSkeleton } from '#/components/access-control/RolesTableSkeleton'
import { cn } from '#/lib/utils'
import { entityColor } from '#/lib/entity-colors'
import { SectionTabNav } from '#/components/SectionTabNav'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/roles')({
  component: WorkspaceRolesRoute,
  pendingComponent: RoutePending,
})


const workspaceRoleScopes = ['workspace', 'environment', 'connection'] as const satisfies readonly RoleScope[]

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
  const [scopeType, setScopeType] = useState<RoleScope>('workspace')
  const [selectedPermissions, setSelectedPermissions] = useState<Set<Permission>>(new Set())
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; description?: string; scope_type?: string; permissions?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const permissionsCatalog = useQuery(orgPermissionsQueryOptions(orgSlug))
  const permissionDefinitions = permissionDefinitionMap(permissionsCatalog.data?.permission_details)
  const scopePermissionMap = permissionsCatalog.data?.scope_map
  const scopePermissionDetails = permissionsCatalog.data?.scope_details[scopeType] ?? []
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
    toast.error(errorMessage(roles.error, 'Failed to load workspace roles'))
  }, [roles.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }
    toast.error(errorMessage(effectivePermissions.error, 'Failed to load role permissions'))
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!permissionsCatalog.error) {
      return
    }
    toast.error(errorMessage(permissionsCatalog.error, 'Failed to load permission catalog'))
  }, [permissionsCatalog.error])

  const createRole = useMutation({
    mutationFn: async () =>
      api.post<Role>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/roles`, {
        name: roleName.trim(),
        description: description.trim(),
        scope_type: scopeType,
        permissions: Array.from(selectedPermissions),
      }),
    onSuccess: async () => {
      setIsCreating(false)
      resetCreateRole()
      toast.success('Role created')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspaceRolesScope(orgSlug, workspaceId) })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          name: error.fieldErrors?.name,
          description: error.fieldErrors?.description,
          scope_type: error.fieldErrors?.scope_type,
          permissions: error.fieldErrors?.permissions,
        })
        if (error.fieldErrors?.name || error.fieldErrors?.description || error.fieldErrors?.scope_type || error.fieldErrors?.permissions) {
          return
        }
      }
      toast.error(errorMessage(error, 'Failed to create role'))
    },
  })

  const deleteRole = useMutation({
    mutationFn: async (roleId: number) => api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/roles/${roleId}`),
    onSuccess: async () => {
      toast.success('Role deleted')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspaceRolesScope(orgSlug, workspaceId) })
    },
    onError: (error) => {
      if (isApiError(error) && error.status === 409) {
        const count = (error.details as { binding_count?: number } | undefined)?.binding_count
        toast.error(
          count != null
            ? `Cannot delete: role has ${count} active policy binding${count !== 1 ? 's' : ''}. Remove them first.`
            : 'Cannot delete: role has active policy bindings. Remove them first.',
        )
        return
      }
      toast.error(errorMessage(error, 'Failed to delete role'))
    },
  })

  function resetCreateRole() {
    setRoleName('')
    setDescription('')
    setScopeType('workspace')
    setSelectedPermissions(new Set())
    setFieldErrors({})
  }

  function submitCreateRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const errors: typeof fieldErrors = {}
    if (!roleName.trim()) {
      errors.name = 'Name is required.'
    }
    if (selectedPermissions.size === 0) {
      errors.permissions = 'Select at least one permission.'
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

  function setScope(nextScope: RoleScope) {
    setScopeType(nextScope)
    setSelectedPermissions((current) => {
      const validPermissions = new Set(scopePermissionMap?.[nextScope] ?? [])
      return new Set(Array.from(current).filter((item) => validPermissions.has(item)))
    })
    setFieldErrors((current) => ({ ...current, scope_type: undefined, permissions: undefined }))
  }

  return (
    <div className="flex flex-col">
      <SectionTabNav
        tabs={[
          { label: 'Policies', to: '/orgs/$org_slug/workspaces/$workspace_id/policies', params: { org_slug: orgSlug, workspace_id: workspaceId }, isActive: false },
          { label: 'Roles', to: '/orgs/$org_slug/workspaces/$workspace_id/roles', params: { org_slug: orgSlug, workspace_id: workspaceId }, isActive: true },
        ]}
      />

      <div className="flex flex-col gap-6 pt-6">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            {!roles.isLoading && total > 0
              ? `${total} workspace role${total !== 1 ? 's' : ''}`
              : 'Workspace-scoped permission sets available for policies.'}
          </p>

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
                <Icon name="plus-sign" size={20} data-icon="inline-start" />
                Create
              </DialogTrigger>
              <DialogContent className="sm:max-w-2xl">
                <DialogHeader>
                  <DialogTitle>Create workspace role</DialogTitle>
                  <DialogDescription>Define a permission set that can be assigned via workspace policies.</DialogDescription>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-6" onSubmit={submitCreateRole}>
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
                    <Label>Scope</Label>
                    <Select
                      items={workspaceRoleScopes.map((scope) => ({ label: scopeLabel(scope), value: scope }))}
                      value={scopeType}
                      onValueChange={(value) => {
                        if (isWorkspaceRoleScope(value)) {
                          setScope(value)
                        }
                      }}
                      disabled={createRole.isPending}
                    >
                      <SelectTrigger className="w-full" aria-invalid={fieldErrors.scope_type ? true : undefined}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {workspaceRoleScopes.map((scope) => (
                            <SelectItem key={scope} value={scope}>
                              {scopeLabel(scope)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    {fieldErrors.scope_type ? <p className="text-sm text-destructive">{fieldErrors.scope_type}</p> : null}
                  </div>

                  <div className="flex flex-col gap-2">
                    <Label htmlFor="workspace-role-description">Description <span className="text-muted-foreground font-normal">(optional)</span></Label>
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
                    key={scopeType}
                    description={`Select the capabilities this role should grant for ${scopeLabel(scopeType).toLowerCase()} resources.`}
                    idPrefix="workspace-permission"
                    selectedPermissions={selectedPermissions}
                    permissionDetails={scopePermissionDetails}
                    permissionDefinitions={permissionDefinitions}
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
                  <TableColumnHeader label="Scope" />
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
              {effectivePermissions.isLoading || roles.isLoading ? <RolesTableSkeleton showActions={canModifyRoles} /> : null}
              {roles.isError ? <TableEmptyState colSpan={canModifyRoles ? 5 : 4} icon="user-shield-01" message="Failed to load roles." /> : null}
              {!effectivePermissions.isLoading && !canReadRoles ? (
                <TableEmptyState colSpan={canModifyRoles ? 5 : 4} icon="user-shield-01" message="You do not have permission to view roles." />
              ) : null}
              {!effectivePermissions.isLoading && canReadRoles && !roles.isLoading && !roles.isError && items.length === 0 ? (
                <TableEmptyState colSpan={canModifyRoles ? 5 : 4} icon="user-shield-01" message={query.q ? 'No roles matched your search.' : 'No roles found.'} />
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
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold', entityColor(role.name))}>
            {role.name.slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{role.name}</div>
            {role.description ? <div className="truncate text-sm text-muted-foreground">{role.description}</div> : null}
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant="outline">{scopeLabel(role.scope_type)}</Badge>
      </TableCell>
      <TableCell>
        <Badge variant={role.is_builtin ? 'secondary' : 'outline'}>{role.is_builtin ? 'System' : 'Custom'}</Badge>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(role.created_at)}</TableCell>
      {canModifyRoles ? (
        <TableCell className="text-end">
          {role.is_builtin ? (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger render={<span className="inline-flex" onClick={(e) => e.stopPropagation()} />}>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled
                    className="pointer-events-none text-muted-foreground/50"
                  >
                    Delete
                  </Button>
                </TooltipTrigger>
                <TooltipContent>System roles cannot be deleted</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : (
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
                    This permanently deletes <strong>{role.name}</strong>. Deletion will fail if any policy bindings still reference this role — remove those bindings first.
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
          )}
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function scopeLabel(value: RoleScope) {
  switch (value) {
    case 'org':
      return 'Organization'
    case 'workspace':
      return 'Workspace'
    case 'environment':
      return 'Environment'
    case 'connection':
      return 'Connection'
  }
}

function isWorkspaceRoleScope(value: string | null): value is RoleScope {
  return workspaceRoleScopes.some((scope) => scope === value)
}
