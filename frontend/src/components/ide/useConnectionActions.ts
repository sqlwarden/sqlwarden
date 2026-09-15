import { useMutation, useQueryClient } from '@tanstack/react-query'
import * as Y from 'yjs'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { errorMessage } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import { invalidateConnectionSchemaQueries } from '#/lib/api/queries/database'
import type { Connection, Workspace } from '#/lib/api/types'
import { invalidateCompletionIndex } from './completion'
import { DEFAULT_CONSOLE_CONTENT, newConnectionTab, useIde } from './useIdeStore'

export function useConnectionActions(orgSlug: string, workspace: Workspace) {
  const openTab = useIde((state) => state.openTab)
  const openConsole = useIde((state) => state.openConsole)
  const sessions = useIde((state) => state.sessions)
  const setSession = useIde((state) => state.setSession)
  const clearSession = useIde((state) => state.clearSession)
  const clearTransactionState = useIde((state) => state.clearTransactionState)
  const setConnectionStatus = useIde((state) => state.setConnectionStatus)
  const queryClient = useQueryClient()
  const sessionsQueryKey = queryKeys.workspaceSessions(orgSlug, workspace.id)

  const connectMutation = useMutation({
    mutationFn: ({ connection }: { connection: Connection; openConsole: boolean }) =>
      api.post<{ session_id: string; reused: boolean }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${connection.id}/connect`,
      ),
    onMutate: ({ connection }) => setConnectionStatus(connection.id, 'connecting'),
    onSuccess: (data, { connection, openConsole: shouldOpenConsole }) => {
      setConnectionStatus(connection.id, null)
      setSession(connection.id, data.session_id)
      void queryClient.invalidateQueries({ queryKey: sessionsQueryKey })
      void invalidateConnectionSchemaQueries(queryClient, orgSlug, workspace.id, connection.id)
      if (shouldOpenConsole) openConnectionConsole(connection)
    },
    onError: (error, { connection }) => {
      const message = errorMessage(error, 'Failed to connect')
      setConnectionStatus(connection.id, { error: message })
      toast.error(message)
    },
  })

  const disconnectMutation = useMutation({
    mutationFn: ({ connection, sessionId }: { connection: Connection; sessionId: string }) =>
      api.delete(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${connection.id}/session`,
        { headers: { 'X-Warden-Session': sessionId } },
      ),
    onSuccess: (_, { connection }) => {
      clearSession(connection.id)
      clearTransactionState(connection.id)
      setConnectionStatus(connection.id, null)
      invalidateCompletionIndex(connection.id)
      void queryClient.invalidateQueries({ queryKey: sessionsQueryKey })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to disconnect')),
  })

  function openConnection(connection: Connection) {
    openTab(newConnectionTab(connection, workspace))
  }

  function openConnectionConsole(connection: Connection) {
    const doc = new Y.Doc()
    doc.getText('content').insert(0, DEFAULT_CONSOLE_CONTENT)
    const initialState = Array.from(Y.encodeStateAsUpdate(doc))
    doc.destroy()
    openConsole(workspace, initialState, connection.id, connection.driver, connection.name)
  }

  function connect(connection: Connection, options: { openConsole?: boolean } = {}) {
    const openConsole = options.openConsole ?? true
    void connectMutation.mutateAsync({ connection, openConsole }).catch(() => undefined)
  }

  function disconnect(connection: Connection, transactionOpen: boolean, onBlocked?: () => void) {
    const sessionId = sessions[connection.id]
    if (!sessionId) {
      return
    }
    if (transactionOpen) {
      onBlocked?.()
      return
    }
    void disconnectMutation.mutateAsync({ connection, sessionId }).catch(() => undefined)
  }

  return {
    connect,
    connectMutation,
    connectedIds: new Set(Object.keys(sessions).map(Number)),
    disconnect,
    disconnectMutation,
    openConnection,
    openConnectionConsole,
    sessions,
  }
}
