import { errorMessage } from '#/lib/api/errors'
import { trimTrailingSlash } from '#/lib/utils'
import { formatDate } from '#/lib/format'
import { useEffect, useState, type FormEvent } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import {
  orgEffectivePermissionsQueryOptions,
  orgMembersQueryOptions,
  orgTeamsQueryOptions,
  orgRolesQueryOptions,
  orgPoliciesQueryOptions,
} from '#/lib/api/query'
import type { OrgMember, PolicyBinding, Role, Team } from '#/lib/api/types'
import { hasPermission, permission, protectedOrgPolicyMessage } from '#/lib/permissions'
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
import { Label } from '#/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { SearchComboboxField } from '#/components/access-control/SearchComboboxField'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { cn } from '#/lib/utils'
import { entityColor } from '#/lib/entity-colors'
import { SectionTabNav } from '#/components/SectionTabNav'
import {
  PoliciesTableSkeleton,
  PolicySubjectCell,
  policySubjectDisplayName,
} from '#/components/access-control/PolicyTablePrimitives'

export const Route = createFileRoute('/orgs/$org_slug/policies')({
  component: OrganizationPoliciesRoute,
  pendingComponent: RoutePending,
})


type SubjectType = 'account' | 'team' | 'org_members'

const SUBJECT_TYPE_LABELS: Record<SubjectType, string> = {
  account: 'User',
  team: 'Team',
  org_members: 'All users',
}

function OrganizationPoliciesRoute() {
  const { org_slug: orgSlug } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const listPath = `/orgs/${orgSlug}/policies`

  if (trimTrailingSlash(pathname) !== listPath) {
    return <Outlet />
  }

  return <OrganizationPoliciesPage orgSlug={orgSlug} />
}

function OrganizationPoliciesPage({ orgSlug }: { orgSlug: string }) {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [subjectType, setSubjectType] = useState<SubjectType>('account')
  const [subjectId, setSubjectId] = useState('')
  const [subjectLabel, setSubjectLabel] = useState('')
  const [roleId, setRoleId] = useState('')
  const [roleLabel, setRoleLabel] = useState('')
  const [rolePermissions, setRolePermissions] = useState<string[]>([])
  const [fieldErrors, setFieldErrors] = useState<{ subject?: string; role?: string }>({})

  // Debounced search queries for combobox pickers
  const [memberQ, setMemberQ] = useState('')
  const [teamQ, setTeamQ] = useState('')
  const [roleQ, setRoleQ] = useState('')

  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'desc',
    q: '',
  })

  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const orgId = effectivePermissions.data?.resource_id

  const canReadPolicies = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canModifyPolicies = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)

  const policies = useQuery({
    ...orgPoliciesQueryOptions(orgSlug, query),
    enabled: canReadPolicies,
  })
  const data = policies.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 10)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  const members = useQuery({
    ...orgMembersQueryOptions(orgSlug, { page_size: 20, q: memberQ }),
    enabled: isCreating && subjectType === 'account',
  })
  const teams = useQuery({
    ...orgTeamsQueryOptions(orgSlug, { page_size: 20, q: teamQ }),
    enabled: isCreating && subjectType === 'team',
  })
  const roles = useQuery({
    ...orgRolesQueryOptions(orgSlug, { page_size: 20, q: roleQ, scope: 'org' } as Parameters<typeof orgRolesQueryOptions>[1]),
    enabled: isCreating,
  })

  useEffect(() => {
    if (!policies.error) return
    toast.error(errorMessage(policies.error, 'Failed to load policies'))
  }, [policies.error])

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(errorMessage(effectivePermissions.error, 'Failed to load permissions'))
  }, [effectivePermissions.error])

  const createPolicy = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {
        subject_type: subjectType,
        role_id: Number(roleId),
      }
      if (subjectType === 'org_members') {
        body.subject_id = orgId
      } else {
        body.subject_id = Number(subjectId)
      }
      return api.post<PolicyBinding>(`/api/v1/orgs/${orgSlug}/policies`, body)
    },
    onSuccess: async () => {
      setIsCreating(false)
      resetForm()
      toast.success('Policy binding created')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgPoliciesScope(orgSlug) })
    },
    onError: (error) => {
      const protectedMessage = protectedOrgPolicyMessage(rolePermissions, effectivePermissions.data?.permissions)
      if (isApiError(error) && error.status === 403 && protectedMessage) {
        setFieldErrors((current) => ({ ...current, role: protectedMessage }))
        toast.error(protectedMessage)
        return
      }
      if (isApiError(error)) {
        setFieldErrors({
          subject: error.fieldErrors?.subject_id ?? error.fieldErrors?.subject_type,
          role: error.fieldErrors?.role_id,
        })
        if (error.fieldErrors?.subject_id || error.fieldErrors?.role_id || error.fieldErrors?.subject_type) {
          return
        }
      }
      toast.error(errorMessage(error, 'Failed to create policy binding'))
    },
  })

  const revokePolicy = useMutation({
    mutationFn: async (bindingId: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/policies/${bindingId}`),
    onSuccess: async () => {
      toast.success('Policy binding revoked')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgPoliciesScope(orgSlug) })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to revoke policy binding'))
    },
  })

  function resetForm() {
    setSubjectType('account')
    setSubjectId('')
    setSubjectLabel('')
    setRoleId('')
    setRoleLabel('')
    setRolePermissions([])
    setMemberQ('')
    setTeamQ('')
    setRoleQ('')
    setFieldErrors({})
  }

  function submitCreatePolicy(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const errors: typeof fieldErrors = {}
    if (subjectType !== 'org_members' && !subjectId) {
      errors.subject = subjectType === 'account' ? 'Select a user.' : 'Select a team.'
    }
    if (!roleId) {
      errors.role = 'Select a role.'
    }
    const protectedMessage = protectedOrgPolicyMessage(rolePermissions, effectivePermissions.data?.permissions)
    if (protectedMessage) {
      errors.role = protectedMessage
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      if (protectedMessage) {
        toast.error(protectedMessage)
      }
      return
    }
    setFieldErrors({})
    void createPolicy.mutateAsync().catch(() => {})
  }

  const colSpan = canModifyPolicies ? 4 : 3

  const memberItems = (members.data?.items ?? []).map((m: OrgMember) => ({
    value: String(m.account_id),
    label: m.name || m.email,
    sublabel: m.name ? m.email : undefined,
  }))

  const teamItems = (teams.data?.items ?? []).map((t: Team) => ({
    value: String(t.id),
    label: t.name,
    sublabel: `@${t.slug}`,
  }))

  const roleItems = (roles.data?.items ?? []).map((r: Role) => ({
    value: String(r.id),
    label: r.name,
    sublabel: r.description,
    permissions: r.permissions ?? [],
  }))

  return (
    <div className="flex flex-col">
      <SectionTabNav
        tabs={[
          { label: 'Policies', to: '/orgs/$org_slug/policies', params: { org_slug: orgSlug }, isActive: true },
          { label: 'Roles', to: '/orgs/$org_slug/roles', params: { org_slug: orgSlug }, isActive: false },
        ]}
      />

      <div className="flex flex-col gap-6 pt-6">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            {!policies.isLoading && total > 0
              ? `${total} policy binding${total !== 1 ? 's' : ''}`
              : 'Assign organization roles to users, teams, or all members.'}
          </p>

          {canModifyPolicies ? (
            <Dialog
              open={isCreating}
              onOpenChange={(open) => {
                setIsCreating(open)
                if (!open) resetForm()
              }}
            >
              <DialogTrigger render={<Button />}>
                <Icon name="plus-sign" size={20} data-icon="inline-start" />
                Assign role
              </DialogTrigger>
              <DialogContent className="sm:max-w-md">
                <DialogHeader>
                  <DialogTitle>Assign role</DialogTitle>
                  <DialogDescription>
                    Bind an organization role to a user, team, or all members.
                  </DialogDescription>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-6" onSubmit={submitCreatePolicy}>
                  {/* Subject type */}
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="subject-type">Subject type</Label>
                    <Select
                      value={subjectType}
                      onValueChange={(value) => {
                        if (value) {
                          setSubjectType(value as SubjectType)
                          setSubjectId('')
                          setSubjectLabel('')
                          setFieldErrors((c) => ({ ...c, subject: undefined }))
                        }
                      }}
                      disabled={createPolicy.isPending}
                    >
                      <SelectTrigger id="subject-type" className="w-full">
                        <SelectValue>{SUBJECT_TYPE_LABELS[subjectType]}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="account">User</SelectItem>
                          <SelectItem value="team">Team</SelectItem>
                          <SelectItem value="org_members">All users</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </div>

                  {/* Subject picker */}
                  {subjectType === 'account' ? (
                <SearchComboboxField
                      label="User"
                      placeholder="Select a user…"
                      searchPlaceholder="Search users…"
                      selectedValue={subjectId}
                      selectedLabel={subjectLabel}
                      items={memberItems}
                      isLoading={members.isLoading}
                      error={fieldErrors.subject}
                      disabled={createPolicy.isPending}
                      onChange={(value, label) => {
                        setSubjectId(value)
                        setSubjectLabel(label)
                        setFieldErrors((c) => ({ ...c, subject: undefined }))
                      }}
                      onSearchChange={setMemberQ}
                    />
                  ) : null}

                  {subjectType === 'team' ? (
                <SearchComboboxField
                      label="Team"
                      placeholder="Select a team…"
                      searchPlaceholder="Search teams…"
                      selectedValue={subjectId}
                      selectedLabel={subjectLabel}
                      items={teamItems}
                      isLoading={teams.isLoading}
                      error={fieldErrors.subject}
                      disabled={createPolicy.isPending}
                      onChange={(value, label) => {
                        setSubjectId(value)
                        setSubjectLabel(label)
                        setFieldErrors((c) => ({ ...c, subject: undefined }))
                      }}
                      onSearchChange={setTeamQ}
                    />
                  ) : null}

                  {subjectType === 'org_members' ? (
                    <div className="rounded-md border border-border bg-muted/40 px-4 py-3">
                      <p className="text-sm text-muted-foreground">
                        The selected role will be granted to{' '}
                        <span className="font-medium text-foreground">all current and future users</span> of this organization.
                      </p>
                    </div>
                  ) : null}

                  {/* Role picker */}
                  <SearchComboboxField
                    label="Role"
                    placeholder="Select a role…"
                    searchPlaceholder="Search roles…"
                    selectedValue={roleId}
                    selectedLabel={roleLabel}
                    items={roleItems}
                    isLoading={roles.isLoading}
                    error={fieldErrors.role}
                    disabled={createPolicy.isPending}
                    onChange={(value, label, item) => {
                      setRoleId(value)
                      setRoleLabel(label)
                      setRolePermissions(item.permissions ?? [])
                      setFieldErrors((c) => ({ ...c, role: undefined }))
                    }}
                    onSearchChange={setRoleQ}
                  />

                  <DialogFooter>
                    <DialogClose render={<Button type="button" variant="ghost" disabled={createPolicy.isPending} />}>
                      Cancel
                    </DialogClose>
                    <Button type="submit" disabled={createPolicy.isPending}>
                      {createPolicy.isPending ? 'Assigning…' : 'Assign'}
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
          placeholder="Search policies"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader label="Subject" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Role" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader
                    label="Assigned"
                    sort="created_at"
                    currentSort={query.sort}
                    currentOrder={query.order}
                    onSortChange={toggleSort}
                  />
                </TableHead>
                {canModifyPolicies ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectivePermissions.isLoading || policies.isLoading ? (
                <PoliciesTableSkeleton canModify={canModifyPolicies} />
              ) : null}
              {policies.isError ? (
                <TableEmptyState colSpan={colSpan} icon="user-shield-01" message="Failed to load policies." />
              ) : null}
              {!effectivePermissions.isLoading && !canReadPolicies ? (
                <TableEmptyState colSpan={colSpan} icon="user-shield-01" message="You do not have permission to view policies." />
              ) : null}
              {!effectivePermissions.isLoading && canReadPolicies && !policies.isLoading && !policies.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={colSpan}
                  icon="user-shield-01"
                  message={query.q ? 'No policies matched your search.' : 'No policy bindings found.'}
                />
              ) : null}
              {!effectivePermissions.isLoading && canReadPolicies && !policies.isLoading && !policies.isError
                ? items.map((binding) => (
                    <PolicyRow
                      key={binding.binding_id}
                      binding={binding}
                      orgSlug={orgSlug}
                      canModify={canModifyPolicies}
                      isRevoking={revokePolicy.isPending}
                      onRevoke={(id) => revokePolicy.mutate(id)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {canReadPolicies && !policies.isLoading && !policies.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="bindings"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={policies.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}
      </div>
    </div>
  )
}

// ─── Combobox field ────────────────────────────────────────────────────────────

// ─── Table row ─────────────────────────────────────────────────────────────────

function PolicyRow({
  binding,
  orgSlug,
  canModify,
  isRevoking,
  onRevoke,
}: {
  binding: PolicyBinding
  orgSlug: string
  canModify: boolean
  isRevoking: boolean
  onRevoke: (bindingId: number) => void
}) {
  const navigate = useNavigate()

  function openBinding() {
    void navigate({
      to: '/orgs/$org_slug/policies/$binding_id',
      params: { org_slug: orgSlug, binding_id: String(binding.binding_id) },
    })
  }

  return (
    <TableRow
      className="cursor-pointer"
      tabIndex={0}
      role="link"
      onClick={openBinding}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          openBinding()
        }
      }}
    >
      <TableCell>
        <PolicySubjectCell binding={binding} />
      </TableCell>
      <TableCell>
        {binding.role_id ? (
          <Link
            to="/orgs/$org_slug/roles/$role_id"
            params={{ org_slug: orgSlug, role_id: String(binding.role_id) }}
            onClick={(e) => e.stopPropagation()}
            className={cn(
              'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium transition-opacity hover:opacity-80',
              entityColor(binding.role_name ?? ''),
            )}
          >
            {binding.role_name ? binding.role_name : '—'}
          </Link>
        ) : (
          <span className={cn('inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium', entityColor(binding.role_name ?? ''))}>
            {binding.role_name ? binding.role_name : '—'}
          </span>
        )}
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(binding.created_at)}</TableCell>
      {canModify ? (
        <TableCell className="text-end">
          <AlertDialog>
            <AlertDialogTrigger
              render={
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={isRevoking}
                  onClick={(e) => e.stopPropagation()}
                />
              }
            >
              Revoke
            </AlertDialogTrigger>
            <AlertDialogContent size="sm" onClick={(e) => e.stopPropagation()}>
              <AlertDialogHeader>
                <AlertDialogTitle>Revoke policy binding?</AlertDialogTitle>
                <AlertDialogDescription>
                  This will remove the{' '}
                  <span className="font-medium">{binding.role_name ? binding.role_name : 'role'}</span>{' '}
                  binding from <span className="font-medium">{subjectDisplayName(binding)}</span>. They will lose any permissions granted by this role.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel variant="ghost" disabled={isRevoking}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  disabled={isRevoking}
                  onClick={() => onRevoke(binding.binding_id)}
                >
                  Revoke
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </TableCell>
      ) : null}
    </TableRow>
  )
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

export function subjectDisplayName(binding: PolicyBinding): string {
  return policySubjectDisplayName(binding)
}




export { entityColor as roleColor, entityColor as subjectColor }
