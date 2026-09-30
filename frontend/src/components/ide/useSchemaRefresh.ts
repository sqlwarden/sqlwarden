import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { errorMessage } from '#/lib/api/errors'
import type { NavigatorRefreshResponse, ScopePath } from '#/lib/api/types'
import {
  applyNavigatorListings,
  connectionObjectDefinitionQueryKeyPrefix,
  connectionObjectsQueryKeyPrefix,
  connectionRelationshipsQueryKeyPrefix,
  refreshSchemaNodes,
} from '#/lib/api/query'
import { invalidateCompletionIndex } from './completion'

/**
 * Refreshes whatever the server has cached at and beneath a path, replaces
 * those listings in place, and drops listings for objects that disappeared.
 * `mutate()` targets the hook's `path` (the connection root when omitted);
 * `mutate(target)` targets another path. Object details are invalidated rather
 * than rewritten because the refresh response carries listings only.
 */
export function useSchemaRefresh({
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  path = [],
}: {
  orgSlug: string
  workspaceId: string | number
  connectionId: string | number
  sessionId?: string
  path?: ScopePath
}) {
  const queryClient = useQueryClient()
  const targetOf = (target: ScopePath | void) => (Array.isArray(target) ? target : path)

  return useMutation<NavigatorRefreshResponse, Error, ScopePath | void>({
    mutationKey: [
      'refresh-connection-schema',
      orgSlug,
      String(workspaceId),
      String(connectionId),
      JSON.stringify(path),
    ],
    mutationFn: (target) =>
      refreshSchemaNodes(orgSlug, workspaceId, connectionId, targetOf(target), sessionId),
    onSuccess: async (result, target) => {
      applyNavigatorListings(
        queryClient,
        orgSlug,
        workspaceId,
        connectionId,
        result.items,
        targetOf(target),
      )
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: connectionObjectsQueryKeyPrefix(orgSlug, workspaceId, connectionId),
        }),
        queryClient.invalidateQueries({
          queryKey: connectionObjectDefinitionQueryKeyPrefix(orgSlug, workspaceId, connectionId),
        }),
        queryClient.invalidateQueries({
          queryKey: connectionRelationshipsQueryKeyPrefix(orgSlug, workspaceId, connectionId),
        }),
      ])
      invalidateCompletionIndex(Number(connectionId))
      toast.success('Schema refreshed')
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to refresh schema'))
    },
  })
}
