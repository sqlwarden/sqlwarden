import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type { ListQuery, Paginated, SessionResponse, SetupStatusResponse, Workspace, Environment, Connection, Organization, InstanceAdmin, Account, AccountOrganization, EffectivePermissions, PermissionsCatalog, ResourceType, OrgMember, WorkspaceMember, WorkspaceEffectiveMember, WorkspaceTeam, Team, TeamMember, Role, PolicyBinding } from '#/lib/api/types'

export const queryKeys = {
  setupStatus: () => ['setup-status'] as const,
  session: () => ['session'] as const,
  accountOrganizations: (query?: ListQuery) => ['account-organizations', query ?? {}] as const,
  instanceAccounts: (query?: ListQuery) => ['instance-accounts', query ?? {}] as const,
  instanceAdmins: (query?: ListQuery) => ['instance-admins', query ?? {}] as const,
  instanceOrganizations: (query?: ListQuery) => ['instance-organizations', query ?? {}] as const,
  orgEffectivePermissions: (slug: string, resourceType: ResourceType, resourceId?: string | number) =>
    ['org-effective-permissions', slug, resourceType, resourceId ?? null] as const,
  orgPermissions: (slug: string) => ['org-permissions', slug] as const,
  org: (slug: string) => ['org', slug] as const,
  orgMembers: (slug: string, query?: ListQuery) => ['org-members', slug, query ?? {}] as const,
  orgMemberCandidates: (slug: string, query?: ListQuery) => ['org-member-candidates', slug, query ?? {}] as const,
  orgMember: (slug: string, accountId: string | number) => ['org-member', slug, accountId] as const,
  orgMemberTeams: (slug: string, accountId: string | number, query?: ListQuery) =>
    ['org-member-teams', slug, accountId, query ?? {}] as const,
  orgTeams: (slug: string, query?: ListQuery) => ['org-teams', slug, query ?? {}] as const,
  orgTeam: (slug: string, teamSlug: string) => ['org-team', slug, teamSlug] as const,
  orgTeamMembers: (slug: string, teamSlug: string, query?: ListQuery) =>
    ['org-team-members', slug, teamSlug, query ?? {}] as const,
  orgRoles: (slug: string, query?: ListQuery) => ['org-roles', slug, query ?? {}] as const,
  orgRole: (slug: string, roleId: string | number) => ['org-role', slug, roleId] as const,
  orgWorkspaceRoles: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-workspace-roles', slug, workspaceId, query ?? {}] as const,
  orgWorkspaceRole: (slug: string, workspaceId: string | number, roleId: string | number) =>
    ['org-workspace-role', slug, workspaceId, roleId] as const,
  orgWorkspaces: (slug: string, query?: ListQuery) => ['org-workspaces', slug, query ?? {}] as const,
  orgWorkspace: (slug: string, workspaceId: string | number) => ['org-workspace', slug, workspaceId] as const,
  orgWorkspaceMembers: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-workspace-members', slug, workspaceId, query ?? {}] as const,
  orgWorkspaceEffectiveMembers: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-workspace-effective-members', slug, workspaceId, query ?? {}] as const,
  orgWorkspaceTeams: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-workspace-teams', slug, workspaceId, query ?? {}] as const,
  orgWorkspacePolicies: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-workspace-policies', slug, workspaceId, query ?? {}] as const,
  myWorkspaces: (query?: ListQuery) => ['my-workspaces', query ?? {}] as const,
  orgEnvironments: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-environments', slug, workspaceId, query ?? {}] as const,
  myEnvironments: (workspaceId: string | number, query?: ListQuery) =>
    ['my-environments', workspaceId, query ?? {}] as const,
  orgConnections: (slug: string, workspaceId: string | number, environmentId: string | number, query?: ListQuery) =>
    ['org-connections', slug, workspaceId, environmentId, query ?? {}] as const,
  orgWorkspaceConnections: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-workspace-connections', slug, workspaceId, query ?? {}] as const,
  myConnections: (workspaceId: string | number, environmentId: string | number, query?: ListQuery) =>
    ['my-connections', workspaceId, environmentId, query ?? {}] as const,
  orgPolicies: (slug: string, query?: ListQuery) => ['org-policies', slug, query ?? {}] as const,
  orgPolicy: (slug: string, bindingId: string | number) => ['org-policy', slug, bindingId] as const,
}

export function setupStatusQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.setupStatus(),
    queryFn: () => api.get<SetupStatusResponse>('/api/setup/status', { skipAuth: true }),
    staleTime: 60_000,
  })
}

export function sessionQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.session(),
    queryFn: () => api.get<SessionResponse>('/api/v1/session'),
    staleTime: 60_000,
  })
}

export function accountOrganizationsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.accountOrganizations(query),
    queryFn: () => api.get<Paginated<AccountOrganization>>('/api/v1/account/orgs', { query }),
    placeholderData: keepPreviousData,
  })
}

export function instanceOrganizationsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.instanceOrganizations(query),
    queryFn: () => api.get<Paginated<Organization>>('/api/v1/instance/orgs', { query }),
    placeholderData: keepPreviousData,
  })
}

export function instanceAccountsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.instanceAccounts(query),
    queryFn: () => api.get<Paginated<Account>>('/api/v1/instance/accounts', { query }),
    placeholderData: keepPreviousData,
  })
}

export function instanceAdminsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.instanceAdmins(query),
    queryFn: () => api.get<Paginated<InstanceAdmin>>('/api/v1/instance/admins', { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgPermissionsQueryOptions(slug: string) {
  return queryOptions({
    queryKey: queryKeys.orgPermissions(slug),
    queryFn: () => api.get<PermissionsCatalog>(`/api/v1/orgs/${slug}/permissions`),
    staleTime: 300_000,
  })
}

export function orgQueryOptions(slug: string) {
  return queryOptions({
    queryKey: queryKeys.org(slug),
    queryFn: () => api.get<Organization>(`/api/v1/orgs/${slug}`),
    staleTime: 60_000,
  })
}

export function orgEffectivePermissionsQueryOptions(slug: string, resourceType: ResourceType, resourceId?: string | number) {
  return queryOptions({
    queryKey: queryKeys.orgEffectivePermissions(slug, resourceType, resourceId),
    queryFn: () =>
      api.get<EffectivePermissions>(`/api/v1/orgs/${slug}/permissions/effective`, {
        query: {
          resource_type: resourceType,
          resource_id: resourceId,
        },
      }),
    staleTime: 60_000,
  })
}

export function orgMembersQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgMembers(slug, query),
    queryFn: () => api.get<Paginated<OrgMember>>(`/api/v1/orgs/${slug}/members`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgMemberCandidatesQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgMemberCandidates(slug, query),
    queryFn: () => api.get<Paginated<Account>>(`/api/v1/orgs/${slug}/members/candidates`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgMemberQueryOptions(slug: string, accountId: string | number) {
  return queryOptions({
    queryKey: queryKeys.orgMember(slug, accountId),
    queryFn: () => api.get<OrgMember>(`/api/v1/orgs/${slug}/members/${accountId}`),
    staleTime: 60_000,
  })
}

export function orgMemberTeamsQueryOptions(slug: string, accountId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgMemberTeams(slug, accountId, query),
    queryFn: () => api.get<Paginated<Team>>(`/api/v1/orgs/${slug}/members/${accountId}/teams`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgTeamsQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgTeams(slug, query),
    queryFn: () => api.get<Paginated<Team>>(`/api/v1/orgs/${slug}/teams`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgTeamQueryOptions(slug: string, teamSlug: string) {
  return queryOptions({
    queryKey: queryKeys.orgTeam(slug, teamSlug),
    queryFn: () => api.get<Team>(`/api/v1/orgs/${slug}/teams/${teamSlug}`),
    staleTime: 60_000,
  })
}

export function orgTeamMembersQueryOptions(slug: string, teamSlug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgTeamMembers(slug, teamSlug, query),
    queryFn: () => api.get<Paginated<TeamMember>>(`/api/v1/orgs/${slug}/teams/${teamSlug}/members`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgRolesQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgRoles(slug, query),
    queryFn: () => api.get<Paginated<Role>>(`/api/v1/orgs/${slug}/roles`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgRoleQueryOptions(slug: string, roleId: string | number) {
  return queryOptions({
    queryKey: queryKeys.orgRole(slug, roleId),
    queryFn: () => api.get<Role>(`/api/v1/orgs/${slug}/roles/${roleId}`),
    staleTime: 60_000,
  })
}

export function orgWorkspaceRolesQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceRoles(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<Role>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/roles`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceRoleQueryOptions(slug: string, workspaceId: string | number, roleId: string | number) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceRole(slug, workspaceId, roleId),
    queryFn: () => api.get<Role>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/roles/${roleId}`),
    staleTime: 60_000,
  })
}

export function orgWorkspacesQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaces(slug, query),
    queryFn: () => api.get<Paginated<Workspace>>(`/api/v1/orgs/${slug}/workspaces`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceQueryOptions(slug: string, workspaceId: string | number) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspace(slug, workspaceId),
    queryFn: () => api.get<Workspace>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}`),
    staleTime: 60_000,
  })
}

export function orgWorkspaceMembersQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceMembers(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<WorkspaceMember>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/users`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceEffectiveMembersQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceEffectiveMembers(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<WorkspaceEffectiveMember>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/users/effective`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceTeamsQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceTeams(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<WorkspaceTeam>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/teams`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspacePoliciesQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspacePolicies(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<PolicyBinding>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/policies`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function myWorkspacesQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.myWorkspaces(query),
    queryFn: () => api.get<Paginated<Workspace>>('/api/v1/me/workspaces', { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgEnvironmentsQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgEnvironments(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<Environment>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/environments`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function myEnvironmentsQueryOptions(workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.myEnvironments(workspaceId, query),
    queryFn: () => api.get<Paginated<Environment>>(`/api/v1/me/workspaces/${workspaceId}/environments`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgConnectionsQueryOptions(
  slug: string,
  workspaceId: string | number,
  environmentId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgConnections(slug, workspaceId, environmentId, query),
    queryFn: () =>
      api.get<Paginated<Connection>>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/environments/${environmentId}/connections`,
        { query },
      ),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceConnectionsQueryOptions(slug: string, workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceConnections(slug, workspaceId, query),
    queryFn: () => api.get<Paginated<Connection>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgPolicyQueryOptions(slug: string, bindingId: string | number) {
  return queryOptions({
    queryKey: queryKeys.orgPolicy(slug, bindingId),
    queryFn: () => api.get<PolicyBinding>(`/api/v1/orgs/${slug}/policies/${bindingId}`),
    staleTime: 60_000,
  })
}

export function orgPoliciesQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgPolicies(slug, query),
    queryFn: () => api.get<Paginated<PolicyBinding>>(`/api/v1/orgs/${slug}/policies`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function myConnectionsQueryOptions(workspaceId: string | number, environmentId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.myConnections(workspaceId, environmentId, query),
    queryFn: () =>
      api.get<Paginated<Connection>>(`/api/v1/me/workspaces/${workspaceId}/environments/${environmentId}/connections`, {
        query,
      }),
    placeholderData: keepPreviousData,
  })
}
