import { useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import {
  createQueryFavorite,
  deleteQueryFavorite,
  updateQueryFavorite,
} from '#/lib/api/queries/query-favorites'
import { queryKeys } from '#/lib/api/query-keys'
import type { OrganizationRuntimeSettings } from '#/lib/api/types'
import { addLocalFavorite, deleteLocalFavorite, updateLocalFavorite } from './localQueryStore'

interface FavoriteInput {
  name: string
  sqlText: string
  connectionId: number | null
}

export function useFavoritesMutations(orgSlug: string, workspaceId: number) {
  const queryClient = useQueryClient()

  const effectiveMode = useCallback((): string => {
    const settings = queryClient.getQueryData<OrganizationRuntimeSettings>(
      orgRuntimeSettingsQueryOptions(orgSlug).queryKey,
    )
    return settings?.effective.query_favorites_mode ?? 'backend'
  }, [orgSlug, queryClient])

  const invalidate = useCallback(
    () =>
      queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceQueryFavoritesScope(orgSlug, workspaceId),
      }),
    [orgSlug, queryClient, workspaceId],
  )

  const create = useCallback(
    async (input: FavoriteInput) => {
      const mode = effectiveMode()
      if (mode === 'off') return
      if (mode === 'backend') {
        await createQueryFavorite(orgSlug, workspaceId, {
          name: input.name,
          sql_text: input.sqlText,
          connection_id: input.connectionId,
        })
      } else {
        await addLocalFavorite({
          workspaceId,
          connectionId: input.connectionId,
          name: input.name,
          sqlText: input.sqlText,
        })
      }
      await invalidate()
    },
    [effectiveMode, invalidate, orgSlug, workspaceId],
  )

  const update = useCallback(
    async (id: number | string, input: FavoriteInput) => {
      const mode = effectiveMode()
      if (mode === 'off') return
      if (mode === 'backend') {
        await updateQueryFavorite(orgSlug, workspaceId, id as number, {
          name: input.name,
          sql_text: input.sqlText,
        })
      } else {
        await updateLocalFavorite(id as string, { name: input.name, sqlText: input.sqlText })
      }
      await invalidate()
    },
    [effectiveMode, invalidate, orgSlug, workspaceId],
  )

  const remove = useCallback(
    async (id: number | string) => {
      const mode = effectiveMode()
      if (mode === 'off') return
      if (mode === 'backend') {
        await deleteQueryFavorite(orgSlug, workspaceId, id as number)
      } else {
        await deleteLocalFavorite(id as string)
      }
      await invalidate()
    },
    [effectiveMode, invalidate, orgSlug, workspaceId],
  )

  return { create, update, remove }
}
