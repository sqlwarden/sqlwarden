import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Cancel01Icon, PlusSignIcon, Search01Icon, UserGroupIcon, UserMultiple02Icon, UserShield01Icon } from '@hugeicons/core-free-icons'
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
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { ScrollArea } from '#/components/ui/scroll-area'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Skeleton } from '#/components/ui/skeleton'
import { TableEmptyState } from '#/components/EmptyState'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { cn } from '#/lib/utils'
import { getInitials } from '#/components/InitialsAvatar'

export const Route = createFileRoute('/orgs/$org_slug/policies')({
  component: OrganizationPoliciesRoute,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
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
    toast.error(policies.error instanceof Error ? policies.error.message : 'Failed to load policies')
  }, [policies.error])

  useEffect(() => {
    if (!effectivePermissions.error) return
    toast.error(effectivePermissions.error instanceof Error ? effectivePermissions.error.message : 'Failed to load permissions')
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
      await queryClient.invalidateQueries({ queryKey: ['org-policies', orgSlug] })
    },
    onError: (error) => {
      if (isApiError(error)) {
        setFieldErrors({
          subject: error.fieldErrors?.subject_id ?? error.fieldErrors?.subject_type,
          role: error.fieldErrors?.role_id,
        })
        if (error.fieldErrors?.subject_id || error.fieldErrors?.role_id || error.fieldErrors?.subject_type) {
          return
        }
      }
      toast.error(error instanceof Error ? error.message : 'Failed to create policy binding')
    },
  })

  const revokePolicy = useMutation({
    mutationFn: async (bindingId: number) =>
      api.delete<void>(`/api/v1/orgs/${orgSlug}/policies/${bindingId}`),
    onSuccess: async () => {
      toast.success('Policy binding revoked')
      await queryClient.invalidateQueries({ queryKey: ['org-policies', orgSlug] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to revoke policy binding')
    },
  })

  function resetForm() {
    setSubjectType('account')
    setSubjectId('')
    setSubjectLabel('')
    setRoleId('')
    setRoleLabel('')
    setMemberQ('')
    setTeamQ('')
    setRoleQ('')
    setFieldErrors({})
  }

  function submitCreatePolicy(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const errors: typeof fieldErrors = {}
    if (subjectType !== 'org_members' && !subjectId) {
      errors.subject = subjectType === 'account' ? 'Select a user' : 'Select a team'
    }
    if (!roleId) {
      errors.role = 'Select a role'
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
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
    label: roleDisplayName(r.name),
    sublabel: r.description,
  }))

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">Policies</h1>
            <p className="text-sm text-muted-foreground">
              {!policies.isLoading && total > 0
                ? `${total} policy binding${total !== 1 ? 's' : ''}`
                : 'Assign organization roles to users, teams, or all members.'}
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
                <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} data-icon="inline-start" />
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
                    <ComboboxField
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
                    <ComboboxField
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
                  <ComboboxField
                    label="Role"
                    placeholder="Select a role…"
                    searchPlaceholder="Search roles…"
                    selectedValue={roleId}
                    selectedLabel={roleLabel}
                    items={roleItems}
                    isLoading={roles.isLoading}
                    error={fieldErrors.role}
                    disabled={createPolicy.isPending}
                    onChange={(value, label) => {
                      setRoleId(value)
                      setRoleLabel(label)
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
                <TableEmptyState colSpan={colSpan} icon={UserShield01Icon} message="Failed to load policies." />
              ) : null}
              {!effectivePermissions.isLoading && !canReadPolicies ? (
                <TableEmptyState colSpan={colSpan} icon={UserShield01Icon} message="You do not have permission to view policies." />
              ) : null}
              {!effectivePermissions.isLoading && canReadPolicies && !policies.isLoading && !policies.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={colSpan}
                  icon={UserShield01Icon}
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
  )
}

// ─── Combobox field ────────────────────────────────────────────────────────────

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
        <PopoverContent
          className="w-(--anchor-width) p-0"
          align="start"
          sideOffset={4}
        >
          {/* Search input */}
          <div className="flex items-center gap-2 border-b border-border px-2">
            <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="size-3.5 shrink-0 text-muted-foreground" />
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(e) => handleSearchChange(e.target.value)}
              placeholder={searchPlaceholder}
              className="h-8 w-full bg-transparent text-xs outline-none placeholder:text-muted-foreground"
            />
            {search ? (
              <button
                type="button"
                onClick={() => { setSearch(''); onSearchChange('') }}
                className="shrink-0 text-muted-foreground hover:text-foreground"
              >
                <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-3" />
              </button>
            ) : null}
          </div>
          {/* Options list */}
          <ScrollArea className="max-h-52">
            <div className="flex flex-col p-1">
              {isLoading ? (
                <div className="flex flex-col gap-1 p-1">
                  {Array.from({ length: 4 }).map((_, i) => (
                    <Skeleton key={i} className="h-7 w-full rounded-md" />
                  ))}
                </div>
              ) : items.length === 0 ? (
                <p className="py-5 text-center text-xs text-muted-foreground">
                  {search ? 'No matches found.' : `No ${label.toLowerCase()}s available.`}
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
        {binding.role_id ? (
          <Link
            to="/orgs/$org_slug/roles/$role_id"
            params={{ org_slug: orgSlug, role_id: String(binding.role_id) }}
            onClick={(e) => e.stopPropagation()}
            className={cn(
              'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium transition-opacity hover:opacity-80',
              roleColor(binding.role_name ?? ''),
            )}
          >
            {binding.role_name ? roleDisplayName(binding.role_name) : '—'}
          </Link>
        ) : (
          <span className={cn('inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium', roleColor(binding.role_name ?? ''))}>
            {binding.role_name ? roleDisplayName(binding.role_name) : '—'}
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
                  <span className="font-medium">{binding.role_name ? roleDisplayName(binding.role_name) : 'role'}</span>{' '}
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

// All subject icons are square (rounded-md) for consistency
function SubjectIcon({ binding }: { binding: PolicyBinding }) {
  if (binding.subject_type === 'account') {
    const initials = getInitials(binding.subject_name, '?')
    return (
      <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold', subjectColor(binding.subject_name))}>
        {initials}
      </div>
    )
  }
  if (binding.subject_type === 'team') {
    return (
      <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-blue-500/10 text-blue-600">
        <HugeiconsIcon icon={UserGroupIcon} strokeWidth={2} className="size-4" />
      </div>
    )
  }
  return (
    <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600">
      <HugeiconsIcon icon={UserMultiple02Icon} strokeWidth={2} className="size-4" />
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
      return <Badge variant="secondary" className="h-4 px-1.5 py-0 text-[10px]">All users</Badge>
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
                <Skeleton className="h-3 w-16" />
              </div>
            </div>
          </TableCell>
          <TableCell>
            <Skeleton className="h-5 w-24 rounded-md" />
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

// ─── Helpers ───────────────────────────────────────────────────────────────────

export function subjectDisplayName(binding: PolicyBinding): string {
  if (binding.subject_type === 'org_members') return 'All users'
  return binding.subject_name || String(binding.subject_id)
}

export function roleDisplayName(name: string): string {
  switch (name) {
    case 'owner': return 'Owner'
    case 'admin': return 'Admin'
    case 'member': return 'Member'
    case 'ws:admin': return 'Workspace Admin'
    case 'ws:member': return 'Workspace Member'
    default: return name
  }
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Unknown'
  return dateFormatter.format(date)
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}

const SUBJECT_COLORS = [
  'bg-violet-500/10 text-violet-600',
  'bg-blue-500/10 text-blue-600',
  'bg-emerald-500/10 text-emerald-600',
  'bg-orange-500/10 text-orange-600',
  'bg-rose-500/10 text-rose-600',
  'bg-amber-500/10 text-amber-600',
  'bg-cyan-500/10 text-cyan-600',
]

function subjectColor(name: string): string {
  if (!name) return SUBJECT_COLORS[0]
  const hash = name.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
  return SUBJECT_COLORS[hash % SUBJECT_COLORS.length]
}

export const ROLE_COLORS = [
  'bg-violet-500/10 text-violet-600',
  'bg-blue-500/10 text-blue-600',
  'bg-emerald-500/10 text-emerald-600',
  'bg-orange-500/10 text-orange-600',
  'bg-rose-500/10 text-rose-600',
  'bg-amber-500/10 text-amber-600',
  'bg-cyan-500/10 text-cyan-600',
]

export function roleColor(name: string): string {
  if (!name) return ROLE_COLORS[0]
  const hash = name.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
  return ROLE_COLORS[hash % ROLE_COLORS.length]
}
