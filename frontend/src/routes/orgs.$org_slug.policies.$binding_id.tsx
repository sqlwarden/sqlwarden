import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { UserGroupIcon, UserMultiple02Icon, UserShield01Icon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import {
  orgEffectivePermissionsQueryOptions,
  orgPermissionsQueryOptions,
  orgPolicyQueryOptions,
  orgRoleQueryOptions,
  orgTeamMembersQueryOptions,
} from '#/lib/api/query'
import type { PermissionDefinition, TeamMember } from '#/lib/api/types'
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
import { InitialsAvatar } from '#/components/InitialsAvatar'
import { RoutePending } from '#/components/RoutePending'
import { Skeleton } from '#/components/ui/skeleton'
import { cn } from '#/lib/utils'
import { roleColor, roleDisplayName, subjectDisplayName } from './orgs.$org_slug.policies'

export const Route = createFileRoute('/orgs/$org_slug/policies/$binding_id')({
  component: PolicyContextPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function PolicyContextPage() {
  const { org_slug: orgSlug, binding_id: bindingId } = Route.useParams()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const canReadPolicies = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canModifyPolicies = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)

  const binding = useQuery({
    ...orgPolicyQueryOptions(orgSlug, bindingId),
    enabled: canReadPolicies,
  })

  const permissionsCatalog = useQuery(orgPermissionsQueryOptions(orgSlug))
  const permissionDefinitions = permissionDefinitionMap(permissionsCatalog.data?.permission_details)

  const role = useQuery({
    ...orgRoleQueryOptions(orgSlug, binding.data?.role_id ?? 0),
    enabled: !!binding.data?.role_id,
  })

  // Fetch team members when subject is a team
  // We pass subject_id (numeric) as the team identifier — backends that accept either slug or ID will work
  const teamMembers = useQuery({
    ...orgTeamMembersQueryOptions(orgSlug, String(binding.data?.subject_id ?? ''), { page_size: 100 }),
    enabled: binding.data?.subject_type === 'team' && !!binding.data?.subject_id,
  })

  const pageTitle = binding.data
    ? `${subjectDisplayName(binding.data)} → ${binding.data.role_name ? roleDisplayName(binding.data.role_name) : 'Policy'}`
    : `Policy #${bindingId}`

  useEffect(() => {
    if (!binding.error) return
    toast.error(binding.error instanceof Error ? binding.error.message : 'Failed to load policy binding')
  }, [binding.error])

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load permissions')
  }, [effectivePermissions.error])

  const revokePolicy = useMutation({
    mutationFn: async () => api.delete<void>(`/api/v1/orgs/${orgSlug}/policies/${bindingId}`),
    onSuccess: async () => {
      toast.success('Policy binding revoked')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['org-policies', orgSlug] }),
        queryClient.invalidateQueries({ queryKey: ['org-policy', orgSlug, bindingId] }),
      ])
      void navigate({ to: '/orgs/$org_slug/policies', params: { org_slug: orgSlug } })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to revoke policy binding')
    },
  })

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-4">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to="/orgs/$org_slug/policies" params={{ org_slug: orgSlug }} />}>
                Policies
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{pageTitle}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">{pageTitle}</h1>
            <p className="text-sm text-muted-foreground">
              Policy binding granting{' '}
              <span className="font-medium text-foreground">
                {binding.data?.role_name ? roleDisplayName(binding.data.role_name) : 'a role'}
              </span>{' '}
              to{' '}
              <span className="font-medium text-foreground">
                {binding.data ? subjectDisplayName(binding.data) : '…'}
              </span>
            </p>
          </div>

          {canModifyPolicies ? (
            <AlertDialog>
              <AlertDialogTrigger render={<Button variant="destructive" disabled={revokePolicy.isPending || !binding.data} />}>
                Revoke
              </AlertDialogTrigger>
              <AlertDialogContent size="sm">
                <AlertDialogHeader>
                  <AlertDialogTitle>Revoke policy binding?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will remove the{' '}
                    <span className="font-medium">{binding.data?.role_name ? roleDisplayName(binding.data.role_name) : 'role'}</span>{' '}
                    binding from{' '}
                    <span className="font-medium">{binding.data ? subjectDisplayName(binding.data) : '…'}</span>.
                    They will lose any permissions granted by this role.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel variant="ghost" disabled={revokePolicy.isPending}>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    disabled={revokePolicy.isPending}
                    onClick={() => revokePolicy.mutate()}
                  >
                    Revoke
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}
        </div>
      </div>

      {/* Binding summary card */}
      <Card className="overflow-hidden">
        {effectivePermissions.isLoading || binding.isLoading ? (
          <div className="flex items-center gap-4 border-b border-border bg-muted/30 px-6 py-5">
            <Skeleton className="size-12 shrink-0 rounded-full" />
            <div className="flex flex-col gap-2">
              <Skeleton className="h-5 w-48" />
              <Skeleton className="h-4 w-32" />
            </div>
          </div>
        ) : null}

        {binding.isError ? (
          <CardContent>
            <ContextMessage message="Failed to load policy binding." />
          </CardContent>
        ) : null}

        {!effectivePermissions.isLoading && !canReadPolicies ? (
          <CardContent>
            <ContextMessage message="You do not have permission to view this policy." />
          </CardContent>
        ) : null}

        {binding.data ? (
          <>
            <div className="flex items-center gap-4 border-b border-border bg-muted/30 px-6 py-5">
              <SubjectIconLarge binding={binding.data} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-xl font-semibold leading-tight tracking-tight">
                    {subjectDisplayName(binding.data)}
                  </h2>
                  <SubjectTypeBadge subjectType={binding.data.subject_type} />
                </div>
                <div className="mt-1">
                  {binding.data.role_id ? (
                    <Link
                      to="/orgs/$org_slug/roles/$role_id"
                      params={{ org_slug: orgSlug, role_id: String(binding.data.role_id) }}
                      className={cn(
                        'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium hover:opacity-80 transition-opacity',
                        roleColor(binding.data.role_name ?? ''),
                      )}
                    >
                      {binding.data.role_name ? roleDisplayName(binding.data.role_name) : '—'}
                    </Link>
                  ) : null}
                </div>
              </div>
            </div>
            <div className="grid gap-5 px-6 py-5 sm:grid-cols-3">
              <InfoBlock label="Binding ID" value={String(binding.data.binding_id)} />
              <InfoBlock label="Resource" value={binding.data.resource_name} />
              <InfoBlock label="Assigned" value={formatDate(binding.data.created_at)} />
            </div>
          </>
        ) : null}
      </Card>

      {/* Team members card — only when subject is a team */}
      {binding.data?.subject_type === 'team' ? (
        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Team members</CardTitle>
          </CardHeader>
          <CardContent>
            {teamMembers.isLoading ? <TeamMembersSkeleton /> : null}
            {teamMembers.isError ? (
              <ContextMessage message="Could not load team members." />
            ) : null}
            {teamMembers.data && teamMembers.data.items.length === 0 ? (
              <ContextMessage message="This team has no members." />
            ) : null}
            {teamMembers.data && teamMembers.data.items.length > 0 ? (
              <div className="flex flex-col divide-y divide-border">
                {teamMembers.data.items.map((member) => (
                  <TeamMemberRow key={member.account_id} member={member} />
                ))}
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      {/* Role permissions card */}
      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>
            {role.data ? (
              <span className="flex items-center gap-2">
                Role permissions
                <Link
                  to="/orgs/$org_slug/roles/$role_id"
                  params={{ org_slug: orgSlug, role_id: String(binding.data?.role_id) }}
                  className={cn(
                    'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium hover:opacity-80 transition-opacity',
                    roleColor(role.data.name),
                  )}
                >
                  {roleDisplayName(role.data.name)}
                </Link>
              </span>
            ) : (
              'Role permissions'
            )}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {binding.isLoading || role.isLoading ? <PermissionGroupsSkeleton /> : null}
          {role.data ? (
            <PermissionGroups
              permissions={(role.data.permissions ?? []) as Permission[]}
              permissionDefinitions={permissionDefinitions}
            />
          ) : null}
          {!binding.isLoading && !role.isLoading && !binding.data?.role_id ? (
            <ContextMessage message="No role associated with this binding." />
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}

function SubjectIconLarge({ binding }: { binding: { subject_type: string; subject_name: string } }) {
  if (binding.subject_type === 'account') {
    return <InitialsAvatar value={binding.subject_name} size="lg" />
  }
  if (binding.subject_type === 'team') {
    return (
      <div className="flex size-12 shrink-0 items-center justify-center rounded-md bg-blue-500/10 text-blue-600">
        <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} className="size-6" />
      </div>
    )
  }
  return (
    <div className="flex size-12 shrink-0 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600">
      <HugeiconsIcon icon={UserMultiple02Icon} strokeWidth={2} className="size-6" />
    </div>
  )
}

function SubjectTypeBadge({ subjectType }: { subjectType: string }) {
  switch (subjectType) {
    case 'account':
      return <Badge variant="outline">User</Badge>
    case 'team':
      return <Badge variant="outline">Team</Badge>
    case 'org_members':
      return <Badge variant="secondary">All users</Badge>
    default:
      return null
  }
}

function TeamMemberRow({ member }: { member: TeamMember }) {
  return (
    <div className="flex items-center gap-3 py-3">
      <InitialsAvatar value={member.name || member.email} size="sm" />
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-foreground">{member.name || member.email}</div>
        {member.name ? <div className="truncate text-xs text-muted-foreground">{member.email}</div> : null}
      </div>
    </div>
  )
}

function TeamMembersSkeleton() {
  return (
    <div className="flex flex-col divide-y divide-border">
      {Array.from({ length: 3 }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 py-3">
          <Skeleton className="size-8 rounded-full shrink-0" />
          <div className="flex flex-col gap-1.5">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-44" />
          </div>
        </div>
      ))}
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
          <p className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">{group.name}</p>
          <div className="flex flex-col gap-3">
            {group.permissions.map((item) => (
              <div key={item}>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-foreground">{permissionDisplayName(item, permissionDefinitions)}</span>
                  <Badge variant="outline" className="h-4 px-1.5 py-0 text-[10px]">{item}</Badge>
                </div>
                {permissionDescription(item, permissionDefinitions) ? (
                  <p className="mt-0.5 text-xs text-muted-foreground">{permissionDescription(item, permissionDefinitions)}</p>
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

function ContextMessage({ message }: { message: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-center">
      <HugeiconsIcon icon={UserShield01Icon} strokeWidth={2} className="size-10 text-muted-foreground" />
      <p className="font-medium text-foreground">{message}</p>
    </div>
  )
}

function InfoBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 border-l-2 border-border pl-3">
      <span className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">{label}</span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
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

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return dateFormatter.format(date)
}
