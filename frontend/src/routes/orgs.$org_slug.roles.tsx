import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Outlet, createFileRoute, useNavigate, useRouter, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Cancel01Icon, PlusSignIcon, Search01Icon, UserShield01Icon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgPermissionsQueryOptions, orgRolesQueryOptions } from '#/lib/api/query'
import type { PermissionDefinition, Role, RoleScope } from '#/lib/api/types'
import { hasPermission, permission, permissionDefinitionMap, permissionDescription, permissionDisplayName, permissionGroupName, scopePermissions, type Permission } from '#/lib/permissions'
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
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '#/components/ui/tooltip'
import { Skeleton } from '#/components/ui/skeleton'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { Textarea } from '#/components/ui/textarea'
import { ScrollArea } from '#/components/ui/scroll-area'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/orgs/$org_slug/roles')({
  component: OrganizationRolesRoute,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function OrganizationRolesRoute() {
  const { org_slug: orgSlug } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const listPath = `/orgs/${orgSlug}/roles`

  if (trimTrailingSlash(pathname) !== listPath) {
    return <Outlet />
  }

  return <OrganizationRolesPage orgSlug={orgSlug} />
}

function OrganizationRolesPage({ orgSlug }: { orgSlug: string }) {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [roleName, setRoleName] = useState('')
  const [description, setDescription] = useState('')
  const [selectedPermissions, setSelectedPermissions] = useState<Set<Permission>>(new Set())
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; description?: string; scope_type?: string; permissions?: string }>({})
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const permissionsCatalog = useQuery(orgPermissionsQueryOptions(orgSlug))
  const permissionDefinitions = permissionDefinitionMap(permissionsCatalog.data?.permission_details)
  const canReadRoles = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canCreateRole = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)
  const canDeleteRole = canCreateRole
  const roles = useQuery({
    ...orgRolesQueryOptions(orgSlug, { ...query, scope: 'org' }),
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

    toast.error(roles.error instanceof Error ? roles.error.message : 'Failed to load roles')
  }, [roles.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }

    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load role permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!permissionsCatalog.error) {
      return
    }

    toast.error(permissionsCatalog.error instanceof Error ? permissionsCatalog.error.message : 'Failed to load permission catalog')
  }, [permissionsCatalog.error])

  const createRole = useMutation({
    mutationFn: async () =>
      api.post<Role>(`/api/v1/orgs/${orgSlug}/roles`, {
        name: roleName.trim(),
        description: description.trim(),
        scope_type: 'org',
        permissions: Array.from(selectedPermissions),
      }),
    onSuccess: async () => {
      setIsCreating(false)
      resetCreateRole()
      toast.success('Role created')
      await queryClient.invalidateQueries({ queryKey: ['org-roles', orgSlug] })
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

      toast.error(error instanceof Error ? error.message : 'Failed to create role')
    },
  })

  const deleteRole = useMutation({
    mutationFn: async (roleId: number) => api.delete<void>(`/api/v1/orgs/${orgSlug}/roles/${roleId}`),
    onSuccess: async () => {
      toast.success('Role deleted')
      await queryClient.invalidateQueries({ queryKey: ['org-roles', orgSlug] })
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
                ? `${total} organization role${total !== 1 ? 's' : ''}`
                : 'Organization-scoped permission sets available for policies.'}
            </p>
          </div>

          {canCreateRole ? (
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
              <DialogContent className="sm:max-w-2xl">
                <DialogHeader>
                  <DialogTitle>Create role</DialogTitle>
                  <DialogDescription>Define a permission set that can be assigned via organization policies.</DialogDescription>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-6" onSubmit={submitCreateRole}>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="role-name">Name</Label>
                    <Input
                      id="role-name"
                      value={roleName}
                      onChange={(event) => {
                        setRoleName(event.target.value)
                        setFieldErrors((current) => ({ ...current, name: undefined }))
                      }}
                      placeholder="database-reader"
                      aria-invalid={fieldErrors.name ? true : undefined}
                      disabled={createRole.isPending}
                    />
                    {fieldErrors.name ? <p className="text-sm text-destructive">{fieldErrors.name}</p> : null}
                  </div>

                  <div className="flex flex-col gap-2">
                    <Label htmlFor="role-description">Description <span className="font-normal text-muted-foreground">(optional)</span></Label>
                    <Textarea
                      id="role-description"
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
                {canDeleteRole ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectivePermissions.isLoading || roles.isLoading ? <RolesTableSkeleton canDeleteRole={canDeleteRole} /> : null}
              {roles.isError ? <TableEmptyState colSpan={canDeleteRole ? 5 : 4} icon={UserShield01Icon} message="Failed to load roles." /> : null}
              {!effectivePermissions.isLoading && !canReadRoles ? (
                <TableEmptyState colSpan={canDeleteRole ? 5 : 4} icon={UserShield01Icon} message="You do not have permission to view roles." />
              ) : null}
              {!effectivePermissions.isLoading && canReadRoles && !roles.isLoading && !roles.isError && items.length === 0 ? (
                <TableEmptyState colSpan={canDeleteRole ? 5 : 4} icon={UserShield01Icon} message={query.q ? 'No roles matched your search.' : 'No roles found.'} />
              ) : null}
              {!effectivePermissions.isLoading && canReadRoles && !roles.isLoading && !roles.isError
                ? items.map((role) => (
                    <RoleRow
                      key={role.id}
                      role={role}
                      canDeleteRole={canDeleteRole}
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
  canDeleteRole,
  isDeleting,
  onDelete,
}: {
  role: Role
  canDeleteRole: boolean
  isDeleting: boolean
  onDelete: (roleId: number) => void
}) {
  const { org_slug: orgSlug } = Route.useParams()
  const router = useRouter()
  const navigate = useNavigate()

  function preloadRole() {
    void router.preloadRoute({
      to: '/orgs/$org_slug/roles/$role_id',
      params: { org_slug: orgSlug, role_id: String(role.id) },
    })
  }

  function openRole() {
    void navigate({
      to: '/orgs/$org_slug/roles/$role_id',
      params: { org_slug: orgSlug, role_id: String(role.id) },
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
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold', roleColor(role.name))}>
            {roleDisplayName(role).slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{roleDisplayName(role)}</div>
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
      {canDeleteRole ? (
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
          )}
        </TableCell>
      ) : null}
    </TableRow>
  )
}

function PermissionPicker({
  selectedPermissions,
  permissionDefinitions,
  disabled,
  error,
  onPermissionChecked,
}: {
  selectedPermissions: Set<Permission>
  permissionDefinitions: ReadonlyMap<string, PermissionDefinition>
  disabled: boolean
  error?: string
  onPermissionChecked: (value: Permission, checked: boolean) => void
}) {
  const [search, setSearch] = useState('')
  const totalPermissions = scopePermissions.org.length
  const groupedPermissions = groupPermissions(scopePermissions.org, permissionDefinitions)

  const filteredGroups = search
    ? groupedPermissions
        .map((group) => ({
          ...group,
          permissions: group.permissions.filter((item) => {
            const q = search.toLowerCase()
            return (
              permissionDisplayName(item, permissionDefinitions).toLowerCase().includes(q) ||
              (permissionDescription(item, permissionDefinitions) ?? '').toLowerCase().includes(q) ||
              item.toLowerCase().includes(q)
            )
          }),
        }))
        .filter((group) => group.permissions.length > 0)
    : groupedPermissions

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-end justify-between gap-4">
        <div className="flex flex-col gap-1">
          <Label>Permissions</Label>
          <p className="text-sm text-muted-foreground">Select the capabilities this role should grant.</p>
        </div>
        {selectedPermissions.size > 0 ? (
          <span className="shrink-0 text-xs text-muted-foreground">{selectedPermissions.size} of {totalPermissions} selected</span>
        ) : null}
      </div>
      <div className="rounded-md border border-border">
        <div className="flex items-center gap-2 border-b border-border px-3">
          <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="size-4 shrink-0 text-muted-foreground" />
          <input
            type="text"
            placeholder="Filter permissions…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          {search ? (
            <button type="button" onClick={() => setSearch('')} className="shrink-0 text-muted-foreground hover:text-foreground">
              <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-3.5" />
            </button>
          ) : null}
        </div>
        <ScrollArea className="h-60">
          <div className="flex flex-col gap-5 p-4">
            {filteredGroups.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">No permissions match your search.</p>
            ) : null}
            {filteredGroups.map((group) => (
              <div key={group.name} className="flex flex-col gap-3">
                <p className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">{group.name}</p>
                <div className="flex flex-col gap-3">
                  {group.permissions.map((item) => {
                    const id = `permission-${item.replace(/[^a-z0-9]+/g, '-')}`
                    const description = permissionDescription(item, permissionDefinitions)
                    return (
                      <label key={item} htmlFor={id} className="flex cursor-pointer items-start gap-2.5">
                        <Checkbox
                          id={id}
                          className="mt-0.5"
                          checked={selectedPermissions.has(item)}
                          disabled={disabled}
                          onCheckedChange={(checked) => onPermissionChecked(item, checked === true)}
                        />
                        <span className="flex flex-col gap-0.5">
                          <span className="text-sm font-medium text-foreground">{permissionDisplayName(item, permissionDefinitions)}</span>
                          {description ? <span className="text-xs text-muted-foreground">{description}</span> : null}
                        </span>
                      </label>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}

function RolesTableSkeleton({ canDeleteRole }: { canDeleteRole: boolean }) {
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
            <Skeleton className="h-5 w-20" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-5 w-16" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          {canDeleteRole ? (
            <TableCell className="text-end">
              <Skeleton className="ms-auto h-8 w-16" />
            </TableCell>
          ) : null}
        </TableRow>
      ))}
    </>
  )
}

function groupPermissions(permissions: readonly Permission[], definitions: ReadonlyMap<string, PermissionDefinition>) {
  const groups = new Map<string, Permission[]>()
  for (const item of permissions) {
    const group = permissionGroupName(item, definitions)
    groups.set(group, [...(groups.get(group) ?? []), item])
  }
  return Array.from(groups.entries()).map(([name, items]) => ({ name, permissions: items }))
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

function roleDisplayName(role: Role) {
  return role.name
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

const ROLE_COLORS = [
  'bg-violet-500/10 text-violet-600',
  'bg-blue-500/10 text-blue-600',
  'bg-emerald-500/10 text-emerald-600',
  'bg-orange-500/10 text-orange-600',
  'bg-rose-500/10 text-rose-600',
  'bg-amber-500/10 text-amber-600',
  'bg-cyan-500/10 text-cyan-600',
]

function roleColor(name: string): string {
  const hash = name.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
  return ROLE_COLORS[hash % ROLE_COLORS.length]
}
