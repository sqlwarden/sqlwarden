import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import {
  orgEffectivePermissionsQueryOptions,
  orgEnvironmentsQueryOptions,
  orgMembersQueryOptions,
  orgQueryOptions,
  orgTeamsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
  orgWorkspacePoliciesQueryOptions,
  orgWorkspaceRolesQueryOptions,
} from '#/lib/api/query'
import type { Connection, Environment, OrgMember, PolicyBinding, ResourceType, Role, Team } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
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
import { Label } from '#/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { ScrollArea } from '#/components/ui/scroll-area'
import { SearchInput } from '#/components/SearchInput'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Skeleton } from '#/components/ui/skeleton'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { getInitials } from '#/components/InitialsAvatar'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/policies')({
  component: WorkspacePoliciesPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

type WorkspaceSubjectType = 'account' | 'team' | 'org_members' | 'workspace_members'
type WorkspacePolicyResourceType = Extract<ResourceType, 'workspace' | 'environment' | 'connection'>

const SUBJECT_TYPE_LABELS: Record<WorkspaceSubjectType, string> = {
  account: 'User',
  team: 'Team',
  org_members: 'All organization users',
  workspace_members: 'All workspace users',
}

function WorkspacePoliciesPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [subjectType, setSubjectType] = useState<WorkspaceSubjectType>('account')
  const [subjectId, setSubjectId] = useState('')
  const [subjectLabel, setSubjectLabel] = useState('')
  const [resourceType, setResourceType] = useState<WorkspacePolicyResourceType>('workspace')
  const [resourceId, setResourceId] = useState('')
  const [resourceLabel, setResourceLabel] = useState('')
  const [roleId, setRoleId] = useState('')
  const [roleLabel, setRoleLabel] = useState('')
  const [fieldErrors, setFieldErrors] = useState<{ subject?: string; resource?: string; role?: string }>({})
  const [memberQ, setMemberQ] = useState('')
  const [teamQ, setTeamQ] = useState('')
  const [roleQ, setRoleQ] = useState('')
  const [environmentQ, setEnvironmentQ] = useState('')
  const [connectionQ, setConnectionQ] = useState('')

  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'created_at',
    order: 'desc',
    q: '',
  })

  const org = useQuery({
    ...orgQueryOptions(orgSlug),
    enabled: isCreating && subjectType === 'org_members',
  })
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const canReadPolicies = hasPermission(effectivePermissions.data?.permissions, permission.policyRead)
  const canModifyPolicies = hasPermission(effectivePermissions.data?.permissions, permission.policyModify)
  const policies = useQuery({
    ...orgWorkspacePoliciesQueryOptions(orgSlug, workspaceId, query),
    enabled: canReadPolicies,
  })
  const members = useQuery({
    ...orgMembersQueryOptions(orgSlug, { page_size: 20, q: memberQ }),
    enabled: isCreating && subjectType === 'account',
  })
  const teams = useQuery({
    ...orgTeamsQueryOptions(orgSlug, { page_size: 20, q: teamQ }),
    enabled: isCreating && subjectType === 'team',
  })
  const roles = useQuery({
    ...orgWorkspaceRolesQueryOptions(orgSlug, workspaceId, { page_size: 100, q: roleQ }),
    enabled: isCreating,
  })
  const environments = useQuery({
    ...orgEnvironmentsQueryOptions(orgSlug, workspaceId, { page_size: 20, q: environmentQ }),
    enabled: isCreating && resourceType === 'environment',
  })
  const connections = useQuery({
    ...orgWorkspaceConnectionsQueryOptions(orgSlug, workspaceId, { page_size: 20, q: connectionQ }),
    enabled: isCreating && resourceType === 'connection',
  })

  const items = policies.data?.items ?? []
  const page = policies.data?.page ?? Number(query.page ?? 1)
  const pageSize = policies.data?.page_size ?? Number(query.page_size ?? 10)
  const total = policies.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1
  const colSpan = canModifyPolicies ? 5 : 4

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load policy permissions')
  }, [effectivePermissions.error])

  useEffect(() => {
    if (!canReadPolicies || !policies.error) return
    toast.error(policies.error instanceof Error ? policies.error.message : 'Failed to load workspace policies')
  }, [canReadPolicies, policies.error])

  useEffect(() => {
    if (!isCreating || !org.error) return
    toast.error(org.error instanceof Error ? org.error.message : 'Failed to load organization')
  }, [isCreating, org.error])

  const createPolicy = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {
        role_id: Number(roleId),
        subject_type: subjectType,
        subject_id: resolveSubjectId(),
        resource_type: resourceType,
      }
      if (resourceType !== 'workspace') {
        body.resource_id = Number(resourceId)
      }
      return api.post<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/policies`, body)
    },
    onSuccess: async () => {
      setIsCreating(false)
      resetForm()
      toast.success('Policy binding created')
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-policies', orgSlug, workspaceId] })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          subject: error.fieldErrors?.subject_id ?? error.fieldErrors?.subject_type,
          resource: error.fieldErrors?.resource_id ?? error.fieldErrors?.resource_type,
          role: error.fieldErrors?.role_id,
        })
        if (error.fieldErrors?.subject_id || error.fieldErrors?.subject_type || error.fieldErrors?.resource_id || error.fieldErrors?.resource_type || error.fieldErrors?.role_id) {
          return
        }
      }
      toast.error(error instanceof Error ? error.message : 'Failed to create policy binding')
    },
  })

  const revokePolicy = useMutation({
    mutationFn: async (bindingId: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/policies/${bindingId}`),
    onSuccess: async () => {
      toast.success('Policy binding revoked')
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-policies', orgSlug, workspaceId] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to revoke policy binding')
    },
  })

  function resolveSubjectId() {
    if (subjectType === 'workspace_members') return Number(workspaceId)
    if (subjectType === 'org_members') return org.data?.id ?? 0
    return Number(subjectId)
  }

  function resetForm() {
    setSubjectType('account')
    setSubjectId('')
    setSubjectLabel('')
    setResourceType('workspace')
    setResourceId('')
    setResourceLabel('')
    setRoleId('')
    setRoleLabel('')
    setMemberQ('')
    setTeamQ('')
    setRoleQ('')
    setEnvironmentQ('')
    setConnectionQ('')
    setFieldErrors({})
  }

  function submitCreatePolicy(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const errors: typeof fieldErrors = {}
    if (subjectType === 'account' && !subjectId) errors.subject = 'Select a user.'
    if (subjectType === 'team' && !subjectId) errors.subject = 'Select a team.'
    if (subjectType === 'org_members' && !org.data?.id) errors.subject = 'Organization is still loading.'
    if (resourceType !== 'workspace' && !resourceId) errors.resource = `Select a ${resourceLabelFor(resourceType).toLowerCase()}.`
    if (!roleId) errors.role = 'Select a role.'
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      return
    }
    setFieldErrors({})
    void createPolicy.mutateAsync().catch(() => { })
  }

  function updateSubjectType(nextSubjectType: WorkspaceSubjectType) {
    setSubjectType(nextSubjectType)
    setSubjectId('')
    setSubjectLabel('')
    setFieldErrors((current) => ({ ...current, subject: undefined }))
  }

  function updateResourceType(nextResourceType: WorkspacePolicyResourceType) {
    setResourceType(nextResourceType)
    setResourceId('')
    setResourceLabel('')
    setRoleId('')
    setRoleLabel('')
    setFieldErrors((current) => ({ ...current, resource: undefined, role: undefined }))
  }

  const memberItems = (members.data?.items ?? []).map((member: OrgMember) => ({
    value: String(member.account_id),
    label: member.name || member.email,
    sublabel: member.name ? member.email : undefined,
  }))
  const teamItems = (teams.data?.items ?? []).map((team: Team) => ({
    value: String(team.id),
    label: team.name,
    sublabel: `@${team.slug}`,
  }))
  const roleItems = (roles.data?.items ?? [])
    .filter((role: Role) => role.scope_type === resourceType)
    .map((role: Role) => ({
      value: String(role.id),
      label: role.name,
      sublabel: role.description || `${resourceLabelFor(role.scope_type)} role`,
    }))
  const environmentItems = (environments.data?.items ?? []).map((environment: Environment) => ({
    value: String(environment.id),
    label: environment.name,
    sublabel: 'Environment',
  }))
  const connectionItems = (connections.data?.items ?? []).map((connection: Connection) => ({
    value: String(connection.id),
    label: connection.name,
    sublabel: `${connection.driver} connection`,
  }))

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">Policies</h1>
            <p className="text-sm text-muted-foreground">
              {!policies.isLoading && total > 0
                ? `${total} policy binding${total !== 1 ? 's' : ''} in this workspace`
                : 'Assign workspace roles to users, teams, and membership groups.'}
            </p>
          </div>

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
                    Bind a workspace role to a subject for this workspace or one of its child resources.
                  </DialogDescription>
                </DialogHeader>
                <form className="mt-6 flex flex-col gap-6" onSubmit={submitCreatePolicy}>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="subject-type">Subject type</Label>
                    <Select
                      value={subjectType}
                      onValueChange={(value) => {
                        if (isWorkspaceSubjectType(value)) updateSubjectType(value)
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
                          <SelectItem value="org_members">All organization users</SelectItem>
                          <SelectItem value="workspace_members">All workspace users</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </div>

                  {subjectType === 'account' ? (
                    <ComboboxField
                      label="User"
                      placeholder="Select a user..."
                      searchPlaceholder="Search users..."
                      selectedValue={subjectId}
                      selectedLabel={subjectLabel}
                      items={memberItems}
                      isLoading={members.isLoading}
                      error={fieldErrors.subject}
                      disabled={createPolicy.isPending}
                      onChange={(value, label) => {
                        setSubjectId(value)
                        setSubjectLabel(label)
                        setFieldErrors((current) => ({ ...current, subject: undefined }))
                      }}
                      onSearchChange={setMemberQ}
                    />
                  ) : null}

                  {subjectType === 'team' ? (
                    <ComboboxField
                      label="Team"
                      placeholder="Select a team..."
                      searchPlaceholder="Search teams..."
                      selectedValue={subjectId}
                      selectedLabel={subjectLabel}
                      items={teamItems}
                      isLoading={teams.isLoading}
                      error={fieldErrors.subject}
                      disabled={createPolicy.isPending}
                      onChange={(value, label) => {
                        setSubjectId(value)
                        setSubjectLabel(label)
                        setFieldErrors((current) => ({ ...current, subject: undefined }))
                      }}
                      onSearchChange={setTeamQ}
                    />
                  ) : null}

                  {subjectType === 'org_members' || subjectType === 'workspace_members' ? (
                    <div className="rounded-md border border-border bg-muted/40 px-4 py-3">
                      <p className="text-sm text-muted-foreground">
                        The selected role will be granted to{' '}
                        <span className="font-medium text-foreground">
                          {subjectType === 'org_members' ? 'all organization users' : 'all direct workspace users'}
                        </span>
                        .
                      </p>
                    </div>
                  ) : null}

                  <div className="flex flex-col gap-2">
                    <Label htmlFor="resource-type">Resource type</Label>
                    <Select
                      value={resourceType}
                      onValueChange={(value) => {
                        if (isWorkspacePolicyResourceType(value)) updateResourceType(value)
                      }}
                      disabled={createPolicy.isPending}
                    >
                      <SelectTrigger id="resource-type" className="w-full">
                        <SelectValue>{resourceLabelFor(resourceType)}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="workspace">This workspace</SelectItem>
                          <SelectItem value="environment">Environment</SelectItem>
                          <SelectItem value="connection">Connection</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </div>

                  {resourceType === 'environment' ? (
                    <ComboboxField
                      label="Environment"
                      placeholder="Select an environment..."
                      searchPlaceholder="Search environments..."
                      emptyMessage="No environments available in this workspace."
                      selectedValue={resourceId}
                      selectedLabel={resourceLabel}
                      items={environmentItems}
                      isLoading={environments.isLoading}
                      error={fieldErrors.resource}
                      disabled={createPolicy.isPending}
                      onChange={(value, label) => {
                        setResourceId(value)
                        setResourceLabel(label)
                        setFieldErrors((current) => ({ ...current, resource: undefined }))
                      }}
                      onSearchChange={setEnvironmentQ}
                    />
                  ) : null}

                  {resourceType === 'connection' ? (
                    <ComboboxField
                      label="Connection"
                      placeholder="Select a connection..."
                      searchPlaceholder="Search connections..."
                      emptyMessage="No connections available in this workspace."
                      selectedValue={resourceId}
                      selectedLabel={resourceLabel}
                      items={connectionItems}
                      isLoading={connections.isLoading}
                      error={fieldErrors.resource}
                      disabled={createPolicy.isPending}
                      onChange={(value, label) => {
                        setResourceId(value)
                        setResourceLabel(label)
                        setFieldErrors((current) => ({ ...current, resource: undefined }))
                      }}
                      onSearchChange={setConnectionQ}
                    />
                  ) : null}

                  <ComboboxField
                    label="Role"
                    placeholder="Select a role..."
                    searchPlaceholder="Search roles..."
                    emptyMessage={`No roles scoped to ${resourceLabelFor(resourceType).toLowerCase()} available in this workspace.`}
                    selectedValue={roleId}
                    selectedLabel={roleLabel}
                    items={roleItems}
                    isLoading={roles.isLoading}
                    error={fieldErrors.role}
                    disabled={createPolicy.isPending}
                    onChange={(value, label) => {
                      setRoleId(value)
                      setRoleLabel(label)
                      setFieldErrors((current) => ({ ...current, role: undefined }))
                    }}
                    onSearchChange={setRoleQ}
                  />

                  <DialogFooter>
                    <DialogClose render={<Button type="button" variant="ghost" disabled={createPolicy.isPending} />}>
                      Cancel
                    </DialogClose>
                    <Button type="submit" disabled={createPolicy.isPending}>
                      {createPolicy.isPending ? 'Assigning...' : 'Assign'}
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
                  <TableColumnHeader label="Subject" sort="subject_name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Role" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Resource" sort="resource_name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Assigned" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                {canModifyPolicies ? (
                  <TableHead className="text-end">
                    <TableColumnHeader label="Actions" />
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectivePermissions.isLoading || policies.isLoading ? <PoliciesTableSkeleton canModify={canModifyPolicies} /> : null}
              {policies.isError ? <TableEmptyState colSpan={colSpan} icon="user-shield-01" message="Failed to load policies." /> : null}
              {!effectivePermissions.isLoading && !canReadPolicies ? (
                <TableEmptyState colSpan={colSpan} icon="user-shield-01" message="You do not have permission to view workspace policies." />
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
  )
}

type PickerItem = { value: string; label: string; sublabel?: string }

function ComboboxField({
  label,
  placeholder,
  searchPlaceholder,
  selectedValue,
  selectedLabel,
  items,
  isLoading,
  error,
  disabled,
  emptyMessage,
  onChange,
  onSearchChange,
}: {
  label: string
  placeholder: string
  searchPlaceholder: string
  selectedValue: string
  selectedLabel: string
  items: PickerItem[]
  isLoading: boolean
  error?: string
  disabled: boolean
  emptyMessage?: string
  onChange: (value: string, label: string) => void
  onSearchChange: (q: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  function handleSearchChange(value: string) {
    setSearch(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => onSearchChange(value), 300)
  }

  function handleSelect(item: PickerItem) {
    onChange(item.value, item.label)
    setOpen(false)
    setSearch('')
    onSearchChange('')
  }

  function handleOpenChange(nextOpen: boolean) {
    setOpen(nextOpen)
    if (!nextOpen) {
      setSearch('')
      onSearchChange('')
    } else {
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger
          disabled={disabled}
          className={cn(
            'flex h-7 w-full items-center justify-between gap-1.5 rounded-md border border-input bg-input/20 px-2 py-1.5 text-xs/relaxed whitespace-nowrap transition-colors outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-50',
            error && 'border-destructive ring-2 ring-destructive/20',
            !selectedValue && 'text-muted-foreground',
          )}
        >
          <span className="truncate">{selectedValue ? selectedLabel : placeholder}</span>
          <svg className="size-3.5 shrink-0 text-muted-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </PopoverTrigger>
        <PopoverContent className="w-(--anchor-width) p-0" align="start" sideOffset={4}>
          <div className="flex items-center gap-2 border-b border-border px-2">
            <Icon name="search-01" size={20} className="size-3.5 shrink-0 text-muted-foreground" />
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(event) => handleSearchChange(event.target.value)}
              placeholder={searchPlaceholder}
              className="h-8 w-full bg-transparent text-xs outline-none placeholder:text-muted-foreground"
            />
            {search ? (
              <button
                type="button"
                onClick={() => {
                  setSearch('')
                  onSearchChange('')
                }}
                className="shrink-0 text-muted-foreground hover:text-foreground"
              >
                <Icon name="cancel-01" size={20} className="size-3" />
              </button>
            ) : null}
          </div>
          <ScrollArea className="max-h-52">
            <div className="flex flex-col p-1">
              {isLoading ? (
                <div className="flex flex-col gap-1 p-1">
                  {Array.from({ length: 4 }).map((_, index) => (
                    <Skeleton key={index} className="h-7 w-full rounded-md" />
                  ))}
                </div>
              ) : items.length === 0 ? (
                <p className="py-5 text-center text-xs text-muted-foreground">
                  {search ? 'No matches found.' : (emptyMessage ?? `No ${label.toLowerCase()}s available.`)}
                </p>
              ) : (
                items.map((item) => (
                  <button
                    key={item.value}
                    type="button"
                    onClick={() => handleSelect(item)}
                    className={cn(
                      'flex flex-col items-start rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent hover:text-accent-foreground',
                      item.value === selectedValue && 'bg-accent text-accent-foreground',
                    )}
                  >
                    <span className="text-xs font-medium">{item.label}</span>
                    {item.sublabel ? <span className="text-[10px] text-muted-foreground">{item.sublabel}</span> : null}
                  </button>
                ))
              )}
            </div>
          </ScrollArea>
        </PopoverContent>
      </Popover>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}

function PolicyRow({
  binding,
  canModify,
  isRevoking,
  onRevoke,
}: {
  binding: PolicyBinding
  canModify: boolean
  isRevoking: boolean
  onRevoke: (bindingId: number) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <SubjectIcon binding={binding} />
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{subjectDisplayName(binding)}</div>
            <div className="mt-0.5 flex items-center gap-1.5">
              <SubjectTypeBadge subjectType={binding.subject_type} />
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant="secondary">{binding.role_name || 'Role'}</Badge>
      </TableCell>
      <TableCell>
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="truncate font-medium text-foreground">{binding.resource_name || resourceLabelFor(binding.resource_type)}</span>
          <span className="text-xs text-muted-foreground">{resourceLabelFor(binding.resource_type)}</span>
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(binding.created_at)}</TableCell>
      {canModify ? (
        <TableCell className="text-end">
          <AlertDialog>
            <AlertDialogTrigger render={<Button variant="destructive" size="sm" disabled={isRevoking} />}>
              Revoke
            </AlertDialogTrigger>
            <AlertDialogContent size="sm">
              <AlertDialogHeader>
                <AlertDialogTitle>Revoke policy binding?</AlertDialogTitle>
                <AlertDialogDescription>
                  This removes the <span className="font-medium">{binding.role_name || 'role'}</span> binding from{' '}
                  <span className="font-medium">{subjectDisplayName(binding)}</span>.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel variant="ghost" disabled={isRevoking}>Cancel</AlertDialogCancel>
                <AlertDialogAction variant="destructive" disabled={isRevoking} onClick={() => onRevoke(binding.binding_id)}>
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

function SubjectIcon({ binding }: { binding: PolicyBinding }) {
  if (binding.subject_type === 'account') {
    return (
      <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
        {getInitials(binding.subject_name, '?')}
      </div>
    )
  }
  if (binding.subject_type === 'team') {
    return (
      <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Icon name="user-group" size={20} className="size-4" />
      </div>
    )
  }
  return (
    <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
      <Icon name="user-multiple-02" size={20} className="size-4" />
    </div>
  )
}

function SubjectTypeBadge({ subjectType }: { subjectType: PolicyBinding['subject_type'] }) {
  switch (subjectType) {
    case 'account':
      return <Badge variant="outline" className="h-4 px-1.5 py-0 text-[10px]">User</Badge>
    case 'team':
      return <Badge variant="outline" className="h-4 px-1.5 py-0 text-[10px]">Team</Badge>
    case 'org_members':
      return <Badge variant="secondary" className="h-4 px-1.5 py-0 text-[10px]">All org users</Badge>
    case 'workspace_members':
      return <Badge variant="secondary" className="h-4 px-1.5 py-0 text-[10px]">All workspace users</Badge>
  }
}

function PoliciesTableSkeleton({ canModify }: { canModify: boolean }) {
  return (
    <>
      {Array.from({ length: 5 }).map((_, index) => (
        <TableRow key={index}>
          <TableCell>
            <div className="flex items-center gap-3">
              <Skeleton className="size-8 rounded-md" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-4 w-36" />
                <Skeleton className="h-3 w-20" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-5 w-24 rounded-md" />
          </TableCell>
          <TableCell>
            <div className="flex flex-col gap-2">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-3 w-20" />
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          {canModify ? (
            <TableCell className="text-end">
              <Skeleton className="ms-auto h-8 w-16" />
            </TableCell>
          ) : null}
        </TableRow>
      ))}
    </>
  )
}

function subjectDisplayName(binding: PolicyBinding): string {
  switch (binding.subject_type) {
    case 'org_members':
      return 'All organization users'
    case 'workspace_members':
      return 'All workspace users'
    default:
      return binding.subject_name || String(binding.subject_id)
  }
}

function resourceLabelFor(value: ResourceType) {
  switch (value) {
    case 'org':
      return 'Organization'
    case 'workspace':
      return 'This workspace'
    case 'environment':
      return 'Environment'
    case 'connection':
      return 'Connection'
  }
}

function isWorkspaceSubjectType(value: string | null): value is WorkspaceSubjectType {
  return value === 'account' || value === 'team' || value === 'org_members' || value === 'workspace_members'
}

function isWorkspacePolicyResourceType(value: string | null): value is WorkspacePolicyResourceType {
  return value === 'workspace' || value === 'environment' || value === 'connection'
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Unknown'
  return dateFormatter.format(date)
}
