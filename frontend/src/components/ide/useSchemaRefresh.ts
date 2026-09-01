import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { errorMessage } from '#/lib/api/errors'
import type { ObjectRef } from '#/lib/api/types'
import {
  connectionObjectQueryKey,
  invalidateConnectionSchemaQueries,
  refreshConnectionSchema,
} from '#/lib/api/query'
import { invalidateCompletionIndex } from './completion'

export function useSchemaRefresh({
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  ref,
}: {
  orgSlug: string
  workspaceId: string | number
  connectionId: string | number
  sessionId?: string
  ref?: ObjectRef
}) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['refresh-connection-schema', orgSlug, String(workspaceId), String(connectionId)],
    mutationFn: () =>
      refreshConnectionSchema(orgSlug, workspaceId, connectionId, sessionId ?? '', ref),
    onSuccess: async (result) => {
      if (result.mode === 'ephemeral' && ref) {
        await queryClient.invalidateQueries({
          queryKey: connectionObjectQueryKey(orgSlug, workspaceId, connectionId, ref),
        })
      } else {
        await invalidateConnectionSchemaQueries(queryClient, orgSlug, workspaceId, connectionId)
      }
      invalidateCompletionIndex(Number(connectionId))
      toast.success('Schema refreshed')
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to refresh schema'))
    },
  })
}
