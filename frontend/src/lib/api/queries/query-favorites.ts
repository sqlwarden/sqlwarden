import { queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import { queryKeys } from '#/lib/api/query-keys'
import type { QueryFavorite } from '#/lib/api/types'

function favoritesBase(slug: string, workspaceId: string | number) {
  return `/api/v1/orgs/${slug}/workspaces/${workspaceId}/query-favorites`
}

export function orgWorkspaceQueryFavoritesQueryOptions(
  slug: string,
  workspaceId: string | number,
  search?: string,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceQueryFavorites(slug, workspaceId, search),
    queryFn: async () => {
      const res = await api.get<{ items: QueryFavorite[] }>(favoritesBase(slug, workspaceId), {
        query: search ? { q: search } : undefined,
      })
      return res.items
    },
  })
}

export function createQueryFavorite(
  slug: string,
  workspaceId: string | number,
  input: { name: string; sql_text: string; connection_id?: number | null },
) {
  return api.post<QueryFavorite>(favoritesBase(slug, workspaceId), input)
}

export function updateQueryFavorite(
  slug: string,
  workspaceId: string | number,
  favoriteId: string | number,
  input: { name: string; sql_text: string },
) {
  return api.patch<QueryFavorite>(`${favoritesBase(slug, workspaceId)}/${favoriteId}`, input)
}

export function deleteQueryFavorite(
  slug: string,
  workspaceId: string | number,
  favoriteId: string | number,
) {
  return api.delete<void>(`${favoritesBase(slug, workspaceId)}/${favoriteId}`)
}
