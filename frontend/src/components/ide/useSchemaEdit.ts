import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { errorMessage } from '#/lib/api/errors'
import type { SchemaEditRequest } from '#/lib/api/types'
import { applyConnectionSchemaEdit, invalidateConnectionSchemaQueries } from '#/lib/api/query'
import { toTransactionState } from './transactionState'
import { useIde } from './useIdeStore'

/**
 * Applies a structured schema mutation and refreshes every cache the change
 * can affect (directory, object detail, relationships) so the explorer
 * updates without a reload. Callers gate availability via
 * schemaEditCapability before ever calling mutate — this hook assumes the
 * request already passed that check and only guards the missing-session case
 * a stale UI could still trigger.
 */
export function useSchemaEdit({
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
}: {
  orgSlug: string
  workspaceId: string | number
  connectionId: string | number
  sessionId?: string
}) {
  const queryClient = useQueryClient()
  const setTransactionState = useIde((state) => state.setTransactionState)

  return useMutation({
    mutationFn: (input: SchemaEditRequest) => {
      if (!sessionId) {
        return Promise.reject(new Error('Connect to the database to make this change.'))
      }
      return applyConnectionSchemaEdit(orgSlug, workspaceId, connectionId, sessionId, input)
    },
    onSuccess: async (result) => {
      setTransactionState(Number(connectionId), toTransactionState(result.transaction))
      await invalidateConnectionSchemaQueries(queryClient, orgSlug, workspaceId, connectionId)
      if (result.schema.status === 'refresh_failed') {
        toast.warning(
          'Change applied, but refreshing the schema snapshot failed. Use Refresh to see the update.',
        )
      }
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to apply schema change'))
    },
  })
}
