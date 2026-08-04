import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  EffectivePermissions,
  ListQuery,
  Organization,
  OrgMember,
  OrganizationInvitation,
  Paginated,
  PermissionsCatalog,
  PolicyBinding,
  ResourceType,
  Role,
  Team,
  TeamMember,
} from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

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

export function orgEffectivePermissionsQueryOptions(
  slug: string,
  resourceType: ResourceType,
  resourceId?: string | number,
) {
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

export function orgInvitationsQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgInvitations(slug, query),
    queryFn: () =>
      api.get<Paginated<OrganizationInvitation>>(`/api/v1/orgs/${slug}/invitations`, { query }),
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

export function orgMemberTeamsQueryOptions(
  slug: string,
  accountId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgMemberTeams(slug, accountId, query),
    queryFn: () =>
      api.get<Paginated<Team>>(`/api/v1/orgs/${slug}/members/${accountId}/teams`, { query }),
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
    queryFn: () =>
      api.get<Paginated<TeamMember>>(`/api/v1/orgs/${slug}/teams/${teamSlug}/members`, { query }),
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

export function orgWorkspaceRolesQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceRoles(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<Role>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/roles`, { query }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceRoleQueryOptions(
  slug: string,
  workspaceId: string | number,
  roleId: string | number,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceRole(slug, workspaceId, roleId),
    queryFn: () => api.get<Role>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/roles/${roleId}`),
    staleTime: 60_000,
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
