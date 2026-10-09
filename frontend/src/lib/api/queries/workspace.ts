import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  Connection,
  ConnectionDetail,
  ConnectionFieldSpec,
  ConnectionSecretName,
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

export function orgWorkspacePolicyQueryOptions(
  slug: string,
  workspaceId: string | number,
  bindingId: string | number,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspacePolicy(slug, workspaceId, bindingId),
    queryFn: () =>
      api.get<PolicyBinding>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/policies/${bindingId}`,
      ),
    staleTime: 60_000,
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

/** The canonical complete connection list used throughout the editor. */
export function allOrgWorkspaceConnectionsQueryOptions(slug: string, workspaceId: string | number) {
  return orgWorkspaceConnectionsQueryOptions(slug, workspaceId, allWorkspaceConnectionsQuery)
}

export function connectionDetailQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return queryOptions({
    queryKey: queryKeys.connectionDetail(slug, workspaceId, connectionId),
    queryFn: () =>
      api.get<ConnectionDetail>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}`,
      ),
    staleTime: 0,
    gcTime: 0,
  })
}

export function engineConnectionFieldsQueryOptions(driver: string) {
  return queryOptions({
    queryKey: queryKeys.engineConnectionFields(driver),
    queryFn: async () =>
      (
        await api.get<{ fields: ConnectionFieldSpec[] }>(
          `/api/v1/engines/${driver}/connection-fields`,
        )
      ).fields,
    staleTime: Infinity,
  })
}

/** Requests one stored secret's value. The response is `Cache-Control: no-store`;
 *  callers keep the value in transient component state only. */
export async function revealConnectionSecret(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  name: ConnectionSecretName,
): Promise<string> {
  const { value } = await api.post<{ value: string }>(
    `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/secrets/${name}/reveal`,
  )
  return value
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
