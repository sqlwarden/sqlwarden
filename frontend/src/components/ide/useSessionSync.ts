import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import { orgWorkspaceConnectionsQueryOptions } from '#/lib/api/query'
import type { Workspace } from '#/lib/api/types'
import { useIde } from './useIdeStore'

export function workspaceSessionsQueryKey(orgSlug: string, workspaceId: number) {
  return ['org-workspace-sessions', orgSlug, workspaceId] as const
}

/**
 * Keeps the persisted sessions map honest for one workspace. The map survives
 * page reloads, so a returning user would otherwise see "connected" dots and
 * stale session ids for server sessions that expired while they were away.
 * Polls the authoritative backend list (and refetches on window focus) and
 * reconciles it into the store, scoped to this workspace's connections so
 * other workspaces' sessions are left alone.
 */
export function useSessionSync(orgSlug: string, workspace: Workspace) {
  const syncSessions = useIde((s) => s.syncSessions)

  const connections = useQuery(
    orgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  const sessionsQuery = useQuery({
    queryKey: workspaceSessionsQueryKey(orgSlug, workspace.id),
    queryFn: () =>
      api.get<{ sessions: { connection_id: number; session_id: string }[] }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/sessions`,
      ),
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
  })

  useEffect(() => {
    if (!sessionsQuery.data || !connections.data) return
    const map: Record<number, string> = {}
    for (const s of sessionsQuery.data.sessions) {
      map[s.connection_id] = s.session_id
    }
    syncSessions(map, connections.data.items.map((c) => c.id))
  }, [sessionsQuery.data, connections.data, syncSessions])
}
