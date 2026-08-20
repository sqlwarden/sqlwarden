import { useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import { recordQueryHistoryEntry } from '#/lib/api/queries/query-history'
import { queryKeys } from '#/lib/api/query-keys'
import type { OrganizationRuntimeSettings } from '#/lib/api/types'
import { addLocalHistoryEntry } from './localQueryStore'

interface RecordHistoryInput {
  sqlText: string
  status: 'ok' | 'error' | 'cancelled'
  errorMessage?: string
  durationMs: number
  rowsAffected: number
}

export function useHistoryRecorder(orgSlug: string, workspaceId: number, connectionId: number) {
  const queryClient = useQueryClient()

  return useCallback(
    (entry: RecordHistoryInput) => {
      const settings = queryClient.getQueryData<OrganizationRuntimeSettings>(
        orgRuntimeSettingsQueryOptions(orgSlug).queryKey,
      )
      const mode = settings?.effective.query_history_mode ?? 'backend'

      if (mode === 'off') return

      if (mode === 'backend') {
        void recordQueryHistoryEntry(orgSlug, workspaceId, connectionId, {
          sql_text: entry.sqlText,
          status: entry.status,
          error_message: entry.errorMessage,
          duration_ms: entry.durationMs,
          rows_affected: entry.rowsAffected,
        })
          .then(() =>
            queryClient.invalidateQueries({
              queryKey: queryKeys.orgWorkspaceQueryHistoryScope(orgSlug, workspaceId),
            }),
          )
          .catch(() => {})
        return
      }

      const retentionCount = settings?.effective.query_history_retention_count ?? 200
      void addLocalHistoryEntry(
        {
          connectionId,
          sqlText: entry.sqlText,
          status: entry.status,
          errorMessage: entry.errorMessage,
          durationMs: entry.durationMs,
          rowsAffected: entry.rowsAffected,
        },
        retentionCount,
      )
        .then(() =>
          queryClient.invalidateQueries({
            queryKey: queryKeys.localQueryHistoryScope(workspaceId),
          }),
        )
        .catch(() => {})
    },
    [connectionId, orgSlug, queryClient, workspaceId],
  )
}
