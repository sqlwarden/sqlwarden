import { errorMessage, isApiError } from '#/lib/api/errors'
import { usePageTitle, usePageTitleScope } from '#/lib/page-title'
import { formatDate } from '#/lib/format'
import { useEffect, useState, type FormEvent } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import {
  orgEffectivePermissionsQueryOptions,
  orgPermissionsQueryOptions,
  orgRoleQueryOptions,
} from '#/lib/api/query'
import type { PermissionDefinition, Role } from '#/lib/api/types'
import {
  hasPermission,
  permission,
  permissionDefinitionMap,
  permissionDescription,
  permissionDisplayName,
  permissionGroupName,
  type Permission,
} from '#/lib/permissions'
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
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '#/components/ui/breadcrumb'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
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
import { Label } from '#/components/ui/label'
import { RoutePending } from '#/components/RoutePending'
import { Skeleton } from '#/components/ui/skeleton'
import { Textarea } from '#/components/ui/textarea'
import { PermissionPicker } from '#/components/access-control/PermissionPicker'
import { cn } from '#/lib/utils'
import { roleScopeLabel } from '#/components/access-control/roleScope'

export const Route = createFileRoute('/orgs/$org_slug/roles/$role_id')({
  component: OrganizationRoleContextPage,
  pendingComponent: RoutePending,
})

function OrganizationRoleContextPage() {
  const { org_slug: orgSlug, role_id: roleId } = Route.useParams()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canReadRole = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canDeleteRole = hasPermission(
    effectivePermissions.data?.permissions,
    permission.policyModify,
  )
  const permissionsCatalog = useQuery(orgPermissionsQueryOptions(orgSlug))
  const permissionDefinitions = permissionDefinitionMap(permissionsCatalog.data?.permission_details)
  const orgScopePermissions = permissionsCatalog.data?.scope_details.org ?? []
  const role = useQuery({
    ...orgRoleQueryOptions(orgSlug, roleId),
    enabled: canReadRole,
  })
  const displayName = role.data ? role.data.name : `Role #${roleId}`
  const { organizationName } = usePageTitleScope()
  usePageTitle(displayName, organizationName)

  const [isEditing, setIsEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editPermissions, setEditPermissions] = useState<Set<Permission>>(new Set())
  const [fieldErrors, setFieldErrors] = useState<{
    name?: string
    description?: string
    permissions?: string
  }>({})

  useEffect(() => {
    if (!role.error) {
      return
    }

    toast.error(errorMessage(role.error, 'Failed to load role'))
  }, [role.error])

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

  const deleteRole = useMutation({
    mutationFn: async () => api.delete<void>(`/api/v1/orgs/${orgSlug}/roles/${roleId}`),
    onSuccess: async () => {
      toast.success('Role deleted')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.orgRolesScope(orgSlug) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.orgRole(orgSlug, roleId) }),
      ])
      void navigate({ to: '/orgs/$org_slug/roles', params: { org_slug: orgSlug } })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to delete role'))
    },
  })

  const updateRole = useMutation({
    mutationFn: async () =>
      api.patch<Role>(`/api/v1/orgs/${orgSlug}/roles/${roleId}`, {
        name: editName.trim(),
        description: editDescription.trim(),
        permissions: Array.from(editPermissions),
      }),
    onSuccess: async () => {
      setIsEditing(false)
      toast.success('Role updated')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.orgRolesScope(orgSlug) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.orgRole(orgSlug, roleId) }),
      ])
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          name: error.fieldErrors?.name,
          description: error.fieldErrors?.description,
          permissions: error.fieldErrors?.permissions,
        })
        if (
          error.fieldErrors?.name ||
          error.fieldErrors?.description ||
          error.fieldErrors?.permissions
        ) {
          return
        }
      }

      toast.error(errorMessage(error, 'Failed to update role'))
    },
  })

  function openEditRole() {
    if (!role.data) {
      return
    }
    setEditName(role.data.name)
    setEditDescription(role.data.description ?? '')
    setEditPermissions(new Set((role.data.permissions ?? []) as Permission[]))
    setFieldErrors({})
    setIsEditing(true)
  }

  function submitEditRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const errors: typeof fieldErrors = {}
    if (!editName.trim()) {
      errors.name = 'Name is required.'
    }
    if (editPermissions.size === 0) {
      errors.permissions = 'Select at least one permission.'
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      return
    }

    setFieldErrors({})
    void updateRole.mutateAsync().catch(() => {})
  }

  function setEditPermissionChecked(value: Permission, checked: boolean) {
    setEditPermissions((current) => {
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
      <div className="flex flex-col gap-4">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink
                render={<Link to="/orgs/$org_slug/roles" params={{ org_slug: orgSlug }} />}
              >
                Roles
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{displayName}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            {role.data ? (
              <div
                className={cn(
                  'flex size-10 shrink-0 items-center justify-center rounded-md text-sm font-semibold',
                  roleColor(role.data.name),
                )}
              >
                {displayName.slice(0, 2).toUpperCase()}
              </div>
            ) : (
              <Skeleton className="size-10 shrink-0 rounded-md" />
            )}
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="font-heading text-2xl font-semibold tracking-tight">
                  {displayName}
                </h1>
                {role.data ? (
                  <>
                    <Badge variant={role.data.is_builtin ? 'secondary' : 'outline'}>
                      {role.data.is_builtin ? 'System' : 'Custom'}
                    </Badge>
                    <Badge variant="outline">{roleScopeLabel(role.data.scope_type)}</Badge>
                  </>
                ) : null}
              </div>
              {role.data?.description ? (
                <p className="mt-0.5 text-sm text-muted-foreground">{role.data.description}</p>
              ) : null}
            </div>
          </div>

          {role.data && canDeleteRole && !role.data.is_builtin ? (
            <div className="flex shrink-0 items-center gap-2">
              <Dialog
                open={isEditing}
                onOpenChange={(open) => {
                  setIsEditing(open)
                  if (open) {
                    openEditRole()
                  }
                }}
              >
                <DialogTrigger render={<Button variant="outline" />}>Edit</DialogTrigger>
                <DialogContent className="sm:max-w-2xl">
                  <DialogHeader>
                    <DialogTitle>Edit role</DialogTitle>
                    <DialogDescription>
                      Update the name, description, and permissions granted by this role.
                    </DialogDescription>
                  </DialogHeader>
                  <form className="mt-6 flex flex-col gap-6" onSubmit={submitEditRole}>
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="edit-role-name">Name</Label>
                      <Input
                        id="edit-role-name"
                        value={editName}
                        onChange={(event) => {
                          setEditName(event.target.value)
                          setFieldErrors((current) => ({ ...current, name: undefined }))
                        }}
                        placeholder="database-reader"
                        aria-invalid={fieldErrors.name ? true : undefined}
                        disabled={updateRole.isPending}
                      />
                      {fieldErrors.name ? (
                        <p className="text-sm text-destructive">{fieldErrors.name}</p>
                      ) : null}
                    </div>

                    <div className="flex flex-col gap-2">
                      <Label htmlFor="edit-role-description">
                        Description{' '}
                        <span className="font-normal text-muted-foreground">(optional)</span>
                      </Label>
                      <Textarea
                        id="edit-role-description"
                        value={editDescription}
                        onChange={(event) => {
                          setEditDescription(event.target.value)
                          setFieldErrors((current) => ({ ...current, description: undefined }))
                        }}
                        placeholder="Describe when this role should be used"
                        aria-invalid={fieldErrors.description ? true : undefined}
                        disabled={updateRole.isPending}
                      />
                      {fieldErrors.description ? (
                        <p className="text-sm text-destructive">{fieldErrors.description}</p>
                      ) : null}
                    </div>

                    <PermissionPicker
                      description="Select the capabilities this role should grant."
                      idPrefix="edit-permission"
                      selectedPermissions={editPermissions}
                      permissionDetails={orgScopePermissions}
                      permissionDefinitions={permissionDefinitions}
                      disabled={updateRole.isPending}
                      error={fieldErrors.permissions}
                      onPermissionChecked={setEditPermissionChecked}
                    />

                    <DialogFooter>
                      <DialogClose
                        render={
                          <Button type="button" variant="ghost" disabled={updateRole.isPending} />
                        }
                      >
                        Cancel
                      </DialogClose>
                      <Button type="submit" disabled={updateRole.isPending}>
                        {updateRole.isPending ? 'Saving...' : 'Save'}
                      </Button>
                    </DialogFooter>
                  </form>
                </DialogContent>
              </Dialog>

              <AlertDialog>
                <AlertDialogTrigger
                  render={<Button variant="destructive" disabled={deleteRole.isPending} />}
                >
                  Delete
                </AlertDialogTrigger>
                <AlertDialogContent size="sm">
                  <AlertDialogHeader>
                    <AlertDialogTitle>Delete role?</AlertDialogTitle>
                    <AlertDialogDescription>
                      This permanently deletes {role.data.name}. Any policies using this role will
                      no longer grant its permissions.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel variant="ghost" disabled={deleteRole.isPending}>
                      Cancel
                    </AlertDialogCancel>
                    <AlertDialogAction
                      variant="destructive"
                      disabled={deleteRole.isPending}
                      onClick={() => deleteRole.mutate()}
                    >
                      Delete
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          ) : null}
        </div>
      </div>

      <Card>
        {role.isError ? (
          <CardContent>
            <ContextMessage message="Failed to load role." />
          </CardContent>
        ) : null}
        {!effectivePermissions.isLoading && !canReadRole ? (
          <CardContent>
            <ContextMessage message="You do not have permission to view this role." />
          </CardContent>
        ) : null}
        {effectivePermissions.isLoading || role.isLoading ? (
          <div className="grid gap-5 px-6 py-5 sm:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="flex flex-col gap-1.5 border-l-2 border-border pl-3">
                <Skeleton className="h-2.5 w-16" />
                <Skeleton className="h-4 w-24" />
              </div>
            ))}
          </div>
        ) : null}
        {role.data ? (
          <div className="grid gap-5 px-6 py-5 sm:grid-cols-3">
            <InfoBlock label="Role ID" value={String(role.data.id)} />
            <InfoBlock label="Created" value={formatDate(role.data.created_at)} />
            <InfoBlock label="Updated" value={formatDate(role.data.updated_at)} />
          </div>
        ) : null}
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Permissions</CardTitle>
        </CardHeader>
        <CardContent>
          {effectivePermissions.isLoading || role.isLoading ? <PermissionGroupsSkeleton /> : null}
          {role.data ? (
            <PermissionGroups
              permissions={(role.data.permissions ?? []) as Permission[]}
              permissionDefinitions={permissionDefinitions}
            />
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}

function PermissionGroups({
  permissions,
  permissionDefinitions,
}: {
  permissions: Permission[]
  permissionDefinitions: ReadonlyMap<string, PermissionDefinition>
}) {
  const groupedPermissions = groupPermissions(permissions, permissionDefinitions)

  if (permissions.length === 0) {
    return <ContextMessage message="This role does not grant any permissions." />
  }

  return (
    <div className="grid gap-6 sm:grid-cols-2">
      {groupedPermissions.map((group) => (
        <div key={group.name} className="flex flex-col gap-3">
          <p className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
            {group.name}
          </p>
          <div className="flex flex-col gap-3">
            {group.permissions.map((item) => (
              <div key={item}>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-foreground">
                    {permissionDisplayName(item, permissionDefinitions)}
                  </span>
                  <Badge variant="outline" className="h-4 px-1.5 py-0 text-[10px]">
                    {item}
                  </Badge>
                </div>
                {permissionDescription(item, permissionDefinitions) ? (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {permissionDescription(item, permissionDefinitions)}
                  </p>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function PermissionGroupsSkeleton() {
  return (
    <div className="grid gap-6 sm:grid-cols-2">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="flex flex-col gap-3">
          <Skeleton className="h-3 w-24" />
          <div className="flex flex-col gap-3">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-4 w-36" />
            <Skeleton className="h-4 w-44" />
          </div>
        </div>
      ))}
    </div>
  )
}

function groupPermissions(
  permissions: readonly Permission[],
  definitions: ReadonlyMap<string, PermissionDefinition>,
) {
  const groups = new Map<string, Permission[]>()
  for (const item of permissions) {
    const group = permissionGroupName(item, definitions)
    groups.set(group, [...(groups.get(group) ?? []), item])
  }
  return Array.from(groups.entries()).map(([name, items]) => ({ name, permissions: items }))
}

function InfoBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 border-l-2 border-border pl-3">
      <span className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
        {label}
      </span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
  )
}

function ContextMessage({ message }: { message: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-center">
      <Icon name="user-shield-01" size={20} className="size-10 text-muted-foreground" />
      <p className="font-medium text-foreground">{message}</p>
    </div>
  )
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
