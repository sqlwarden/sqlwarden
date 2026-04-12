import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type { ListQuery, Paginated, SessionResponse, SetupStatusResponse, Workspace, Environment, Connection } from '#/lib/api/types'

export const queryKeys = {
  setupStatus: () => ['setup-status'] as const,
  session: () => ['session'] as const,
  orgWorkspaces: (slug: string, query?: ListQuery) => ['org-workspaces', slug, query ?? {}] as const,
  myWorkspaces: (query?: ListQuery) => ['my-workspaces', query ?? {}] as const,
  orgEnvironments: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    ['org-environments', slug, workspaceId, query ?? {}] as const,
  myEnvironments: (workspaceId: string | number, query?: ListQuery) =>
    ['my-environments', workspaceId, query ?? {}] as const,
  orgConnections: (slug: string, workspaceId: string | number, environmentId: string | number, query?: ListQuery) =>
    ['org-connections', slug, workspaceId, environmentId, query ?? {}] as const,
  myConnections: (workspaceId: string | number, environmentId: string | number, query?: ListQuery) =>
    ['my-connections', workspaceId, environmentId, query ?? {}] as const,
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

export function orgWorkspacesQueryOptions(slug: string, query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaces(slug, query),
    queryFn: () => api.get<Paginated<Workspace>>(`/api/v1/orgs/${slug}/workspaces`, { query }),
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
