import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { UserShield01Icon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { orgEffectivePermissionsQueryOptions, orgRoleQueryOptions } from '#/lib/api/query'
import type { RoleScope } from '#/lib/api/types'
import { hasPermission, permission, type Permission } from '#/lib/permissions'
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
import { RoutePending } from '#/components/RoutePending'
import { Skeleton } from '#/components/ui/skeleton'

export const Route = createFileRoute('/orgs/$org_slug/roles/$role_id')({
  component: OrganizationRoleContextPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function OrganizationRoleContextPage() {
  const { org_slug: orgSlug, role_id: roleId } = Route.useParams()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canReadRole = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canDeleteRole = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)
  const role = useQuery({
    ...orgRoleQueryOptions(orgSlug, roleId),
    enabled: canReadRole,
  })
  const displayName = role.data ? roleDisplayName(role.data.name) : `Role #${roleId}`

  useEffect(() => {
    if (!role.error) {
      return
    }

    toast.error(role.error instanceof Error ? role.error.message : 'Failed to load role')
  }, [role.error])

  useEffect(() => {
    if (!effectivePermissions.error) {
      return
    }

    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load role permissions')
  }, [effectivePermissions.error])

  const deleteRole = useMutation({
    mutationFn: async () => api.delete<void>(`/api/v1/orgs/${orgSlug}/roles/${roleId}`),
    onSuccess: async () => {
      toast.success('Role deleted')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['org-roles', orgSlug] }),
        queryClient.invalidateQueries({ queryKey: ['org-role', orgSlug, roleId] }),
      ])
      void navigate({ to: '/orgs/$org_slug/roles', params: { org_slug: orgSlug } })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete role')
    },
  })

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-4">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to="/orgs/$org_slug/roles" params={{ org_slug: orgSlug }} />}>
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
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">{displayName}</h1>
            <p className="text-sm text-muted-foreground">
              {role.data?.description || 'Permission set used by organization policies.'}
            </p>
          </div>

          {role.data && canDeleteRole && !role.data.is_builtin ? (
            <AlertDialog>
              <AlertDialogTrigger render={<Button variant="destructive" disabled={deleteRole.isPending} />}>
                Delete
              </AlertDialogTrigger>
              <AlertDialogContent size="sm">
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete role?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This permanently deletes {roleDisplayName(role.data.name)}. Any policies using this role will no longer grant its permissions.
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
          ) : null}
        </div>
      </div>

      <Card className="overflow-hidden">
        {effectivePermissions.isLoading || role.isLoading ? (
          <div className="flex items-center gap-4 border-b border-border bg-muted/30 px-6 py-5">
            <Skeleton className="size-12 shrink-0 rounded-md" />
            <div className="flex flex-col gap-2">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-56" />
            </div>
          </div>
        ) : null}

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

        {role.data ? (
          <>
            <div className="flex items-center gap-4 border-b border-border bg-muted/30 px-6 py-5">
              <div className="flex size-12 shrink-0 items-center justify-center rounded-md bg-muted text-sm font-semibold text-muted-foreground">
                {roleDisplayName(role.data.name).slice(0, 2).toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-xl font-semibold leading-tight tracking-tight">{roleDisplayName(role.data.name)}</h2>
                  <Badge variant={role.data.is_builtin ? 'secondary' : 'outline'}>
                    {role.data.is_builtin ? 'System' : 'Custom'}
                  </Badge>
                  <Badge variant="outline">{scopeLabel(role.data.scope_type)}</Badge>
                </div>
                <p className="mt-0.5 truncate text-sm text-muted-foreground">{role.data.description || 'No description'}</p>
              </div>
            </div>
            <div className="grid gap-5 px-6 py-5 sm:grid-cols-3">
              <InfoBlock label="Role ID" value={String(role.data.id)} />
              <InfoBlock label="Created" value={formatDate(role.data.created_at)} />
              <InfoBlock label="Updated" value={formatDate(role.data.updated_at)} />
            </div>
          </>
        ) : null}
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Permissions</CardTitle>
        </CardHeader>
        <CardContent>
          {effectivePermissions.isLoading || role.isLoading ? <PermissionGroupsSkeleton /> : null}
          {role.data ? <PermissionGroups permissions={(role.data.permissions ?? []) as Permission[]} /> : null}
        </CardContent>
      </Card>
    </div>
  )
}

function PermissionGroups({ permissions }: { permissions: Permission[] }) {
  const groupedPermissions = groupPermissions(permissions)

  if (permissions.length === 0) {
    return <ContextMessage message="This role does not grant any permissions." />
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {groupedPermissions.map((group) => (
        <div key={group.name} className="flex flex-col gap-2 rounded-md border border-border p-4">
          <p className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">{group.name}</p>
          <div className="flex flex-wrap gap-2">
            {group.permissions.map((item) => (
              <Badge key={item} variant="outline">
                {item}
              </Badge>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function PermissionGroupsSkeleton() {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="flex flex-col gap-3 rounded-md border border-border p-4">
          <Skeleton className="h-4 w-28" />
          <div className="flex flex-wrap gap-2">
            <Skeleton className="h-5 w-20" />
            <Skeleton className="h-5 w-24" />
            <Skeleton className="h-5 w-16" />
          </div>
        </div>
      ))}
    </div>
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
    case 'org':
      return 'Organization'
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

function roleDisplayName(value: string) {
  switch (value) {
    case 'owner':
      return 'Owner'
    case 'admin':
      return 'Admin'
    case 'member':
      return 'Member'
    case 'ws:admin':
      return 'Workspace Admin'
    case 'ws:member':
      return 'Workspace Member'
    default:
      return value
  }
}

function InfoBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 border-l-2 border-border pl-3">
      <span className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">{label}</span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
  )
}

function ContextMessage({ message }: { message: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-center">
      <HugeiconsIcon icon={UserShield01Icon} strokeWidth={2} className="size-10 text-muted-foreground" />
      <p className="font-medium text-foreground">{message}</p>
    </div>
  )
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return dateFormatter.format(date)
}
