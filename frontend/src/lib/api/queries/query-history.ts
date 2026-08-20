import { infiniteQueryOptions, keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import { queryKeys } from '#/lib/api/query-keys'
import type { ListQuery, Paginated, QueryHistoryEntry } from '#/lib/api/types'

const WORKSPACE_HISTORY_PAGE_SIZE = 25

function historyBase(slug: string, workspaceId: string | number, connectionId: string | number) {
  return `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/history`
}

function workspaceHistoryBase(slug: string, workspaceId: string | number) {
  return `/api/v1/orgs/${slug}/workspaces/${workspaceId}/history`
}

export function orgWorkspaceQueryHistoryInfiniteQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: number | undefined,
  search?: string,
) {
  return infiniteQueryOptions({
    queryKey: queryKeys.orgWorkspaceQueryHistory(slug, workspaceId, connectionId, search),
    queryFn: ({ pageParam }) =>
      api.get<Paginated<QueryHistoryEntry>>(workspaceHistoryBase(slug, workspaceId), {
        query: {
          page: pageParam,
          page_size: WORKSPACE_HISTORY_PAGE_SIZE,
          ...(connectionId !== undefined ? { connection_id: connectionId } : {}),
          ...(search ? { q: search } : {}),
        },
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.page_size < lastPage.total ? lastPage.page + 1 : undefined,
  })
}

export function orgConnectionQueryHistoryQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  query?: ListQuery,
) {
  return queryOptions({
    queryKey: queryKeys.orgConnectionQueryHistory(slug, workspaceId, connectionId, query),
    queryFn: () =>
      api.get<Paginated<QueryHistoryEntry>>(historyBase(slug, workspaceId, connectionId), {
        query,
      }),
    placeholderData: keepPreviousData,
  })
}

export function recordQueryHistoryEntry(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  entry: {
    sql_text: string
    status: 'ok' | 'error' | 'cancelled'
    error_message?: string | null
    duration_ms: number
    rows_affected: number
  },
) {
  return api.post<QueryHistoryEntry>(historyBase(slug, workspaceId, connectionId), entry)
}

export function deleteQueryHistoryEntry(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  historyId: string | number,
) {
  return api.delete<void>(`${historyBase(slug, workspaceId, connectionId)}/${historyId}`)
}

export function clearQueryHistoryForConnection(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return api.delete<void>(historyBase(slug, workspaceId, connectionId))
}
