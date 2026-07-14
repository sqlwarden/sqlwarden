import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  Connection,
  Environment,
  JobRecord,
  ListQuery,
  Paginated,
  PolicyBinding,
  Workspace,
  WorkspaceEffectiveMember,
  WorkspaceMember,
  WorkspaceTeam,
} from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

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

export function orgWorkspaceMembersQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceMembers(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<WorkspaceMember>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/users`, {
        query,
      }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceEffectiveMembersQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceEffectiveMembers(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<WorkspaceEffectiveMember>>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/users/effective`,
        { query },
      ),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspaceTeamsQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceTeams(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<WorkspaceTeam>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/teams`, {
        query,
      }),
    placeholderData: keepPreviousData,
  })
}

export function orgWorkspacePoliciesQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspacePolicies(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<PolicyBinding>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/policies`, {
        query,
      }),
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

export function orgEnvironmentsQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgEnvironments(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<Environment>>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/environments`,
        { query },
      ),
    placeholderData: keepPreviousData,
  })
}

export function myEnvironmentsQueryOptions(workspaceId: string | number, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.myEnvironments(workspaceId, query),
    queryFn: () =>
      api.get<Paginated<Environment>>(`/api/v1/me/workspaces/${workspaceId}/environments`, {
        query,
      }),
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

export function orgWorkspaceConnectionsQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceConnections(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<Connection>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections`, {
        query,
      }),
    placeholderData: keepPreviousData,
  })
}

const allWorkspaceConnectionsQuery = {
  page_size: 100,
  sort: 'name',
  order: 'asc',
} satisfies ListQuery

/** The canonical complete connection list used throughout the IDE. */
export function allOrgWorkspaceConnectionsQueryOptions(slug: string, workspaceId: string | number) {
  return orgWorkspaceConnectionsQueryOptions(slug, workspaceId, allWorkspaceConnectionsQuery)
}

export function orgWorkspaceJobsQueryOptions(
  slug: string,
  workspaceId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceJobs(slug, workspaceId, query),
    queryFn: () =>
      api.get<Paginated<JobRecord>>(`/api/v1/orgs/${slug}/workspaces/${workspaceId}/jobs`, {
        query,
      }),
    placeholderData: keepPreviousData,
  })
}

export function myConnectionsQueryOptions(
  workspaceId: string | number,
  environmentId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.myConnections(workspaceId, environmentId, query),
    queryFn: () =>
      api.get<Paginated<Connection>>(
        `/api/v1/me/workspaces/${workspaceId}/environments/${environmentId}/connections`,
        {
          query,
        },
      ),
    placeholderData: keepPreviousData,
  })
}
