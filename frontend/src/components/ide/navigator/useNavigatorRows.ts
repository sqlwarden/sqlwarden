import { useEffect, useState } from 'react'
import { partialMatchKey, useQueries, useQueryClient } from '@tanstack/react-query'
import { isSessionRequired } from '#/lib/api/errors'
import { schemaNodesQueryOptions } from '#/lib/api/query'
import { queryKeys } from '#/lib/api/query-keys'
import type { NavigatorListing, NavigatorTree, ScopePath } from '#/lib/api/types'
import { useEvictGoneSession } from '../sessionErrors'
import { useIde } from '../useIdeStore'
import { flattenNavigator, type ListingState } from './model'

interface NavigatorRowsInput {
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId?: string
  tree: NavigatorTree | undefined
  filter: string
}

/**
 * Flattens the navigator from the node-listing query cache and keeps a query
 * observer on every expanded folder. Collapsed folders are read from the cache
 * only (for filtering), so the hook also re-renders on any node-cache write.
 */
export function useNavigatorRows({
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  tree,
  filter,
}: NavigatorRowsInput) {
  const queryClient = useQueryClient()
  const expandedNodes = useIde((s) => s.expandedNodes)
  const setNodeExpanded = useIde((s) => s.setNodeExpanded)
  const prefix = `nav:${connectionId}:`
  useNodeCacheSubscription(orgSlug, workspaceId, connectionId)

  useEffect(() => {
    if (!sessionId) return
    void queryClient.resetQueries({
      queryKey: queryKeys.connectionSchemaNodesScope(orgSlug, workspaceId, connectionId),
      predicate: (query) => query.state.status === 'error',
    })
  }, [queryClient, orgSlug, workspaceId, connectionId, sessionId])

  const listing = (parent: ScopePath, folder: string): ListingState | undefined => {
    const state = queryClient.getQueryState<NavigatorListing>(
      queryKeys.connectionSchemaNodes(orgSlug, workspaceId, connectionId, parent, folder),
    )
    if (!state) return undefined
    if (state.data) return { status: 'ready', listing: state.data }
    if (state.status === 'error') {
      return isSessionRequired(state.error) ? { status: 'session_required' } : { status: 'error' }
    }
    return { status: 'loading' }
  }

  const { rows, requests } = tree
    ? flattenNavigator({
        tree,
        isExpanded: (key) => expandedNodes[prefix + key] === true,
        listing,
        filter,
      })
    : { rows: [], requests: [] }

  const results = useQueries({
    queries: requests.map((request) =>
      schemaNodesQueryOptions(
        orgSlug,
        workspaceId,
        connectionId,
        request.parent,
        request.folder,
        sessionId,
      ),
    ),
  })
  useEvictGoneSession(
    connectionId,
    results.map((result) => result.error),
  )

  return {
    rows,
    toggle: (key: string, expanded: boolean) => setNodeExpanded(prefix + key, expanded),
    retry: (parent: ScopePath, folder: string) =>
      queryClient.resetQueries({
        queryKey: queryKeys.connectionSchemaNodes(
          orgSlug,
          workspaceId,
          connectionId,
          parent,
          folder,
        ),
      }),
  }
}

function useNodeCacheSubscription(orgSlug: string, workspaceId: number, connectionId: number) {
  const queryClient = useQueryClient()
  const [, setVersion] = useState(0)
  useEffect(() => {
    const scope = queryKeys.connectionSchemaNodesScope(orgSlug, workspaceId, connectionId)
    return queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== 'updated' && event.type !== 'removed') return
      if (partialMatchKey(event.query.queryKey, scope)) setVersion((v) => v + 1)
    })
  }, [queryClient, orgSlug, workspaceId, connectionId])
}
