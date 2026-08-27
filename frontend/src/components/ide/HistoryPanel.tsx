import { useCallback, useEffect, useState, type UIEvent } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import {
  allOrgWorkspaceConnectionsQueryOptions,
  orgEnvironmentsQueryOptions,
  orgRuntimeSettingsQueryOptions,
} from '#/lib/api/query'
import { orgWorkspaceQueryFavoritesQueryOptions } from '#/lib/api/queries/query-favorites'
import {
  deleteQueryHistoryEntry,
  orgWorkspaceQueryHistoryInfiniteQueryOptions,
} from '#/lib/api/queries/query-history'
import { queryKeys } from '#/lib/api/query-keys'
import { SearchInput } from '#/components/SearchInput'
import { Button } from '#/components/ui/button'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { copyWithToast } from './contextMenus/clipboard'
import { DriverBadge } from './DriverBadge'
import { ALL_CONNECTIONS, HistoryConnectionSelector } from './HistoryConnectionSelector'
import type { IdeSidebarPanelProps } from './ideActivities'
import { insertAtCursor } from './insertAtCursor'
import {
  listLocalFavorites,
  listLocalHistoryPage,
  type LocalFavorite,
  type LocalHistoryPage,
} from './localQueryStore'
import { formatExactTime, formatRelativeTime } from './relativeTime'
import { SaveFavoriteDialog } from './SaveFavoriteDialog'
import { SidebarPane } from './SidebarPane'
import { Tip } from './schema-diagram/Tip'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useFavoritesMutations } from './useFavoritesMutations'
import { useIde, activeTabId as selectActiveTabId } from './useIdeStore'

function favoriteKey(connectionId: number | null, sqlText: string): string {
  return `${connectionId ?? 'none'}::${sqlText.trim()}`
}

const HISTORY_PAGE_SIZE = 25

type HistoryRow = {
  id: number | string
  connectionId: number
  sqlText: string
  status: 'ok' | 'error' | 'cancelled'
  executedAt: string
}

function statusLabel(status: HistoryRow['status']): string {
  switch (status) {
    case 'ok':
      return 'Succeeded'
    case 'error':
      return 'Failed'
    default:
      return 'Cancelled'
  }
}

function statusColorClass(status: HistoryRow['status']): string {
  switch (status) {
    case 'ok':
      return 'text-green-600 dark:text-green-400'
    case 'error':
      return 'text-destructive'
    default:
      return 'text-muted-foreground'
  }
}

export function HistoryPanel({ orgSlug, workspace }: IdeSidebarPanelProps) {
  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const activeConnectionId = useIde((s) => s.tabs.find((t) => t.id === activeTabId)?.connectionId)
  const viewRegistry = useEditorViewRegistry()

  const [favoriteRow, setFavoriteRow] = useState<HistoryRow | null>(null)
  const [connectionFilter, setConnectionFilter] = useState<number | typeof ALL_CONNECTIONS>(
    () => activeConnectionId ?? ALL_CONNECTIONS,
  )
  const filterConnectionId = connectionFilter === ALL_CONNECTIONS ? undefined : connectionFilter
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText()

  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const mode = runtimeSettings.data?.effective.query_history_mode ?? 'backend'

  const connections = useQuery(allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id))
  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, {
      page_size: 100,
      sort: 'name',
      order: 'asc',
    }),
  )

  const backendQuery = useInfiniteQuery({
    ...orgWorkspaceQueryHistoryInfiniteQueryOptions(
      orgSlug,
      workspace.id,
      filterConnectionId,
      debouncedQuery,
    ),
    enabled: mode === 'backend',
  })

  const localQuery = useInfiniteQuery({
    queryKey: queryKeys.localQueryHistory(workspace.id, filterConnectionId, debouncedQuery),
    queryFn: ({ pageParam }): Promise<LocalHistoryPage> =>
      listLocalHistoryPage({
        connectionId: filterConnectionId,
        search: debouncedQuery || undefined,
        page: pageParam,
        pageSize: HISTORY_PAGE_SIZE,
      }),
    initialPageParam: 1,
    getNextPageParam: (last) =>
      last.page * last.page_size < last.total ? last.page + 1 : undefined,
    enabled: mode === 'local',
  })

  const activeQuery = mode === 'backend' ? backendQuery : localQuery

  const favoritesMutations = useFavoritesMutations(orgSlug, workspace.id)
  const favoritesMode = runtimeSettings.data?.effective.query_favorites_mode ?? 'backend'
  const favoritesQuery = useQuery({
    ...orgWorkspaceQueryFavoritesQueryOptions(orgSlug, workspace.id),
    enabled: favoritesMode === 'backend',
  })
  const [localFavorites, setLocalFavorites] = useState<LocalFavorite[]>([])
  const refreshLocalFavorites = useCallback(() => {
    if (favoritesMode !== 'local') return
    void listLocalFavorites(workspace.id).then(setLocalFavorites)
  }, [favoritesMode, workspace.id])
  useEffect(() => {
    if (favoritesMode !== 'local') {
      setLocalFavorites([])
      return
    }
    refreshLocalFavorites()
  }, [favoritesMode, refreshLocalFavorites])

  const favoriteIdByKey = new Map<string, number | string>(
    favoritesMode === 'backend'
      ? (favoritesQuery.data ?? []).map(
          (fav) => [favoriteKey(fav.connection_id, fav.sql_text), fav.id] as const,
        )
      : localFavorites.map((fav) => [favoriteKey(fav.connectionId, fav.sqlText), fav.id] as const),
  )

  async function handleRemoveFavorite(id: number | string) {
    await favoritesMutations.remove(id)
    refreshLocalFavorites()
  }

  function handleInsert(sqlText: string) {
    if (!activeTabId || !activeGroupId) return
    const view = viewRegistry.get(`${activeGroupId}:${activeTabId}`)
    if (!view) return
    insertAtCursor(view, sqlText)
  }

  function handleCopy(sqlText: string) {
    copyWithToast(sqlText, 'Query copied')
  }

  async function handleDelete(id: number | string, connectionId: number) {
    await deleteQueryHistoryEntry(orgSlug, workspace.id, connectionId, id)
    await backendQuery.refetch()
  }

  function onScroll(e: UIEvent<HTMLDivElement>) {
    if (!activeQuery.hasNextPage || activeQuery.isFetchingNextPage) return
    const el = e.currentTarget
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 200) {
      void activeQuery.fetchNextPage()
    }
  }

  if (mode === 'off') {
    return (
      <SidebarPane title="History" icon="history">
        <div className="px-3 py-4 text-center text-xs text-muted-foreground">
          Query history is turned off for this organization.
        </div>
      </SidebarPane>
    )
  }

  const rows: HistoryRow[] =
    mode === 'backend'
      ? (backendQuery.data?.pages.flatMap((page) => page.items) ?? []).map((entry) => ({
          id: entry.id,
          connectionId: entry.connection_id,
          sqlText: entry.sql_text,
          status: entry.status,
          executedAt: entry.executed_at,
        }))
      : (localQuery.data?.pages.flatMap((page) => page.items) ?? []).map((entry) => ({
          id: entry.id,
          connectionId: entry.connectionId,
          sqlText: entry.sqlText,
          status: entry.status,
          executedAt: entry.executedAt,
        }))

  return (
    <SidebarPane
      title="History"
      icon="history"
      scroll={false}
      headerContent={
        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search query history…"
          className="w-full"
          size="sm"
          variant="muted"
        />
      }
    >
      <div className="flex h-full min-h-0 flex-col">
        <div className="flex shrink-0 flex-col gap-2 border-b border-border p-2">
          <HistoryConnectionSelector
            connections={connections.data?.items ?? []}
            environments={environments.data?.items ?? []}
            isLoading={connections.isLoading}
            value={connectionFilter}
            activeHintConnectionId={activeConnectionId}
            onChange={setConnectionFilter}
          />
        </div>

        <div
          className="min-h-0 flex-1 overflow-y-auto"
          onScroll={onScroll}
          data-testid="history-scroll"
        >
          {activeQuery.isLoading ? (
            <div className="px-3 py-4 text-center text-xs text-muted-foreground">
              <Icon name="loading-03" size={14} className="mx-auto mb-1 animate-spin" />
              Loading history…
            </div>
          ) : rows.length === 0 ? (
            <div className="px-3 py-4 text-center text-xs text-muted-foreground">
              {debouncedQuery ? (
                <>
                  <p className="font-medium text-foreground">No matching queries</p>
                  <p className="mt-0.5">Try a different search term.</p>
                </>
              ) : (
                <>
                  <p className="font-medium text-foreground">No queries run yet</p>
                  <p className="mt-0.5">Queries you run will show up here.</p>
                </>
              )}
            </div>
          ) : (
            <div className="flex flex-col gap-2 p-2">
              {rows.map((row) => {
                const connection = connections.data?.items.find((c) => c.id === row.connectionId)
                const favoriteId = favoriteIdByKey.get(favoriteKey(row.connectionId, row.sqlText))
                const isFavorited = favoriteId !== undefined
                return (
                  <div
                    key={row.id}
                    data-testid="history-row"
                    className="flex flex-col gap-2 rounded-lg border border-border/60 bg-card/40 p-2.5 transition-colors hover:border-border hover:bg-muted/20"
                  >
                    <div className="flex items-center gap-1.5">
                      {connection && (
                        <DriverBadge driver={connection.driver} size="sm" className="size-3" />
                      )}
                      <span className="min-w-0 flex-1 truncate text-xs font-normal text-muted-foreground">
                        {connection?.name ?? 'Unknown connection'}
                      </span>
                      <span
                        className={cn(
                          'shrink-0 text-[10px] font-medium',
                          statusColorClass(row.status),
                        )}
                      >
                        {statusLabel(row.status)}
                      </span>
                    </div>

                    <div className="max-h-16 overflow-y-auto rounded-md bg-muted/40 px-2 py-1.5">
                      <code className="block whitespace-pre-wrap break-words font-mono text-xs leading-snug text-foreground">
                        {row.sqlText}
                      </code>
                    </div>

                    <div className="flex items-center justify-between gap-1">
                      <Tip label={formatExactTime(row.executedAt)}>
                        <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
                          {formatRelativeTime(row.executedAt)}
                        </span>
                      </Tip>
                      <div className="flex items-center gap-1">
                        <Tip label={isFavorited ? 'Remove from favorites' : 'Save as favorite'}>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            aria-label={isFavorited ? 'Remove from favorites' : 'Save as favorite'}
                            onClick={() => {
                              if (favoriteId !== undefined) {
                                void handleRemoveFavorite(favoriteId)
                              } else {
                                setFavoriteRow(row)
                              }
                            }}
                            className={
                              isFavorited ? 'text-amber-500 dark:text-amber-400' : undefined
                            }
                          >
                            {isFavorited ? (
                              <svg
                                viewBox="0 0 24 24"
                                width={12}
                                height={12}
                                fill="currentColor"
                                aria-hidden="true"
                              >
                                <path d="M10.788 3.21c.448-1.077 1.976-1.077 2.424 0l2.082 5.007 5.404.433c1.164.093 1.636 1.545.749 2.305l-4.117 3.527 1.257 5.273c.271 1.136-.964 2.033-1.96 1.425L12 18.354 7.373 21.18c-.996.608-2.231-.29-1.96-1.425l1.257-5.273-4.117-3.527c-.887-.76-.415-2.212.749-2.305l5.404-.433 2.082-5.007Z" />
                              </svg>
                            ) : (
                              <Icon name="star" size={12} />
                            )}
                          </Button>
                        </Tip>
                        <Tip label="Copy query">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Copy query"
                            onClick={() => handleCopy(row.sqlText)}
                          >
                            <Icon name="copy-01" size={12} />
                          </Button>
                        </Tip>
                        <Tip label="Insert at cursor">
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Insert query at cursor"
                            onClick={() => handleInsert(row.sqlText)}
                          >
                            <Icon name="text-cursor" size={12} />
                          </Button>
                        </Tip>
                        {mode === 'backend' && (
                          <Tip label="Delete from history">
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-sm"
                              aria-label="Delete history entry"
                              onClick={() => void handleDelete(row.id, row.connectionId)}
                            >
                              <Icon name="delete-01" size={12} />
                            </Button>
                          </Tip>
                        )}
                      </div>
                    </div>
                  </div>
                )
              })}
              {activeQuery.isFetchingNextPage && (
                <div className="flex items-center justify-center gap-1.5 py-2 text-[10px] text-muted-foreground">
                  <Icon name="loading-03" size={12} className="animate-spin" />
                  Loading more…
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <SaveFavoriteDialog
        open={favoriteRow !== null}
        onOpenChange={(open) => {
          if (!open) setFavoriteRow(null)
        }}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        sqlText={favoriteRow?.sqlText ?? ''}
        connectionId={favoriteRow?.connectionId ?? null}
        onSaved={refreshLocalFavorites}
      />
    </SidebarPane>
  )
}
