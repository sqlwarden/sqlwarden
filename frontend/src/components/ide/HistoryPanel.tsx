import { useEffect, useRef, useState, type UIEvent } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { toast } from 'sonner'
import {
  allOrgWorkspaceConnectionsQueryOptions,
  orgEnvironmentsQueryOptions,
  orgRuntimeSettingsQueryOptions,
} from '#/lib/api/query'
import { orgWorkspaceQueryFavoritesQueryOptions } from '#/lib/api/queries/query-favorites'
import {
  clearQueryHistoryForConnection,
  deleteQueryHistoryEntry,
  orgWorkspaceQueryHistoryInfiniteQueryOptions,
} from '#/lib/api/queries/query-history'
import { queryKeys } from '#/lib/api/query-keys'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '#/components/ui/alert-dialog'
import { SearchInput } from '#/components/SearchInput'
import { Button } from '#/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { Icon } from '#/lib/icons'
import { copyWithToast } from './contextMenus/clipboard'
import { DriverBadge } from './DriverBadge'
import { ALL_CONNECTIONS, HistoryConnectionSelector } from './HistoryConnectionSelector'
import { insertAtCursor } from './insertAtCursor'
import { ReadOnlySqlView } from './object-detail/ReadOnlySqlView'
import { highlightSqlStatic } from './object-detail/staticSqlHighlight'
import type { BottomPanelTabProps } from './bottomPanels'
import {
  clearLocalHistory,
  listLocalFavorites,
  listLocalHistoryPage,
  type LocalFavorite,
  type LocalHistoryPage,
} from './localQueryStore'
import { formatDateGroup, formatExactTime } from './relativeTime'
import { SaveFavoriteDialog } from './SaveFavoriteDialog'
import { SidebarPane } from './SidebarPane'
import { Tip } from './schema-diagram/Tip'
import { isExpandableSql, flattenSql } from './sqlPreview'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useFavoritesMutations } from './useFavoritesMutations'
import { useIde, activeTabId as selectActiveTabId } from './useIdeStore'
import { IdeEmptyState } from './IdeEmptyState'

function favoriteKey(connectionId: number | null, sqlText: string): string {
  return `${connectionId ?? 'none'}::${sqlText.trim()}`
}

const HISTORY_PAGE_SIZE = 25
const DELETE_UNDO_WINDOW_MS = 5000
const DIVIDER_ROW_HEIGHT = 24
const DATA_ROW_HEIGHT = 36
const HISTORY_COLUMN_COUNT = 5

type HistoryRow = {
  id: number | string
  connectionId: number
  sqlText: string
  executedAt: string
}

type HistoryListItem =
  | { type: 'divider'; key: string; label: string }
  | { type: 'row'; row: HistoryRow; rowNumber: number }

function buildHistoryListItems(rows: HistoryRow[]): HistoryListItem[] {
  const items: HistoryListItem[] = []
  let lastGroup: string | null = null
  rows.forEach((row, i) => {
    const group = formatDateGroup(row.executedAt)
    if (group !== lastGroup) {
      items.push({ type: 'divider', key: `divider-${group}`, label: group })
      lastGroup = group
    }
    items.push({ type: 'row', row, rowNumber: i + 1 })
  })
  return items
}

function HistoryEmptyState({ filtered = false }: { filtered?: boolean }) {
  return (
    <IdeEmptyState
      icon={filtered ? 'search-01' : 'history'}
      title={filtered ? 'No matching queries' : 'No queries run yet'}
      description={filtered ? 'Try a different search term.' : 'Queries you run will show up here.'}
    />
  )
}

function HistoryColumnWidths() {
  return (
    <colgroup>
      <col className="w-9" />
      <col className="w-36" />
      <col />
      <col className="w-48" />
      <col className="w-32" />
    </colgroup>
  )
}

function HistoryTableHeader() {
  return (
    <TableHeader className="sticky top-0 z-10 hidden bg-muted/20 @lg:table-header-group">
      <TableRow className="hover:bg-transparent">
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          #
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Connection
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Query
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Run at
        </TableHead>
        <TableHead className="text-right text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Actions
        </TableHead>
      </TableRow>
    </TableHeader>
  )
}

function HistoryDateDividerRow({
  label,
  dataIndex,
  measureRef,
}: {
  label: string
  dataIndex: number
  measureRef: (el: HTMLTableRowElement | null) => void
}) {
  return (
    <TableRow data-index={dataIndex} ref={measureRef} className="hover:bg-transparent">
      <TableCell
        colSpan={HISTORY_COLUMN_COUNT}
        className="bg-muted/10 py-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground/60"
      >
        {label}
      </TableCell>
    </TableRow>
  )
}

type HistoryRowItemProps = {
  row: HistoryRow
  index: number
  dataIndex: number
  measureRef: (el: HTMLTableRowElement | null) => void
  connectionName: string | undefined
  driver: string | undefined
  isFavorited: boolean
  canDelete: boolean
  expanded: boolean
  onToggleExpand: () => void
  onToggleFavorite: () => void
  onCopy: () => void
  onInsertAtCursor: () => void
  onDelete: () => void
}

function HistoryRowItem({
  row,
  index,
  dataIndex,
  measureRef,
  connectionName,
  driver,
  isFavorited,
  canDelete,
  expanded,
  onToggleExpand,
  onToggleFavorite,
  onCopy,
  onInsertAtCursor,
  onDelete,
}: HistoryRowItemProps) {
  const expandable = isExpandableSql(row.sqlText)
  const cellAlign = expanded ? 'align-top' : 'align-middle'

  return (
    <TableRow
      data-testid="history-row"
      data-index={dataIndex}
      ref={measureRef}
      className={`group cursor-grab active:cursor-grabbing ${cellAlign}`}
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', row.sqlText)
        e.dataTransfer.effectAllowed = 'copy'
      }}
    >
      <TableCell className={cellAlign}>
        <span className="relative flex size-4 items-center justify-center text-[10px] text-muted-foreground tabular-nums">
          <span className="group-hover:opacity-0">{index}</span>
          <Icon
            name="drag-handle"
            size={12}
            className="absolute inset-0 m-auto opacity-0 group-hover:opacity-100"
          />
        </span>
      </TableCell>

      <TableCell className={cellAlign}>
        <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-muted-foreground">
          {driver && <DriverBadge driver={driver} size="sm" className="size-3 shrink-0" />}
          <span className="truncate">{connectionName ?? 'Unknown connection'}</span>
        </span>
      </TableCell>

      <TableCell className={cellAlign}>
        {expanded ? (
          <div className="flex min-w-0 items-start gap-1">
            <button
              type="button"
              onClick={onToggleExpand}
              aria-expanded={expanded}
              aria-label="Collapse query"
              className="mt-1 shrink-0 text-muted-foreground hover:text-foreground"
            >
              <Icon name="chevron-down" size={10} />
            </button>
            <div className="min-w-0 flex-1 overflow-hidden rounded-sm border border-border bg-muted/30">
              <ReadOnlySqlView
                value={row.sqlText}
                wrap={false}
                className="max-h-64 overflow-auto"
              />
            </div>
          </div>
        ) : expandable ? (
          <button
            type="button"
            onClick={onToggleExpand}
            aria-expanded={expanded}
            aria-label="Expand query"
            className="flex w-full min-w-0 items-center gap-1 rounded-sm text-left font-mono text-xs leading-snug text-foreground hover:text-foreground/80"
          >
            <Icon name="chevron-right" size={10} className="shrink-0 text-muted-foreground" />
            <span className="truncate">{highlightSqlStatic(flattenSql(row.sqlText))}</span>
          </button>
        ) : (
          <span className="block min-w-0 truncate font-mono text-xs leading-snug text-foreground">
            {highlightSqlStatic(flattenSql(row.sqlText))}
          </span>
        )}
      </TableCell>

      <TableCell className={cellAlign}>
        <span className="truncate text-[10px] text-muted-foreground tabular-nums">
          {formatExactTime(row.executedAt)}
        </span>
      </TableCell>

      <TableCell className={`${cellAlign} text-right`}>
        <div className="flex shrink-0 items-center justify-end gap-1">
          <Tip label={isFavorited ? 'Remove from favorites' : 'Save as favorite'}>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={isFavorited ? 'Remove from favorites' : 'Save as favorite'}
              onClick={onToggleFavorite}
              className={isFavorited ? 'text-amber-500 dark:text-amber-400' : undefined}
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
              onClick={onCopy}
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
              onClick={onInsertAtCursor}
            >
              <Icon name="text-cursor" size={12} />
            </Button>
          </Tip>
          {canDelete && (
            <Tip label="Delete from history">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Delete history entry"
                onClick={onDelete}
              >
                <Icon name="delete-01" size={12} />
              </Button>
            </Tip>
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

export function HistoryPanel({
  orgSlug,
  workspace,
  isMaximized,
  onMaximize,
  onClose,
}: BottomPanelTabProps) {
  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeConnectionId = useIde((s) => s.tabs.find((t) => t.id === activeTabId)?.connectionId)
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const viewRegistry = useEditorViewRegistry()

  const [favoriteRow, setFavoriteRow] = useState<HistoryRow | null>(null)
  const [expandedIds, setExpandedIds] = useState<Set<number | string>>(new Set())
  const [pendingDeleteIds, setPendingDeleteIds] = useState<Set<number | string>>(new Set())
  const [clearAllOpen, setClearAllOpen] = useState(false)
  const [clearAllPending, setClearAllPending] = useState(false)
  const deleteTimers = useRef(new Map<number | string, ReturnType<typeof setTimeout>>())
  const scrollRef = useRef<HTMLDivElement>(null)
  const [connectionFilter, setConnectionFilter] = useState<number | typeof ALL_CONNECTIONS>(
    () => activeConnectionId ?? ALL_CONNECTIONS,
  )
  const filterConnectionId = connectionFilter === ALL_CONNECTIONS ? undefined : connectionFilter
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText()

  useEffect(
    () => () => {
      deleteTimers.current.forEach((timer) => clearTimeout(timer))
    },
    [],
  )

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
  function refreshLocalFavorites() {
    if (favoritesMode !== 'local') return
    void listLocalFavorites(workspace.id).then(setLocalFavorites)
  }
  useEffect(() => {
    if (favoritesMode !== 'local') {
      setLocalFavorites([])
      return
    }
    refreshLocalFavorites()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [favoritesMode, workspace.id])

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

  function handleCopy(sqlText: string) {
    copyWithToast(sqlText, 'Query copied')
  }

  function handleInsertAtCursor(row: HistoryRow) {
    if (!activeTabId || !activeGroupId) return
    const view = viewRegistry.get(`${activeGroupId}:${activeTabId}`)
    if (!view) return
    insertAtCursor(view, row.sqlText)
  }

  function toggleExpand(id: number | string) {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function scheduleDelete(row: HistoryRow) {
    setPendingDeleteIds((prev) => new Set(prev).add(row.id))
    const timer = setTimeout(() => {
      deleteTimers.current.delete(row.id)
      void deleteQueryHistoryEntry(orgSlug, workspace.id, row.connectionId, row.id).then(() =>
        backendQuery.refetch(),
      )
    }, DELETE_UNDO_WINDOW_MS)
    deleteTimers.current.set(row.id, timer)
    toast('Query deleted from history', {
      action: {
        label: 'Undo',
        onClick: () => {
          const pendingTimer = deleteTimers.current.get(row.id)
          if (pendingTimer) clearTimeout(pendingTimer)
          deleteTimers.current.delete(row.id)
          setPendingDeleteIds((prev) => {
            const next = new Set(prev)
            next.delete(row.id)
            return next
          })
        },
      },
    })
  }

  async function handleClearAll() {
    if (!filterConnectionId) return
    setClearAllPending(true)
    try {
      if (mode === 'backend') {
        await clearQueryHistoryForConnection(orgSlug, workspace.id, filterConnectionId)
        await backendQuery.refetch()
      } else {
        await clearLocalHistory(filterConnectionId)
        await localQuery.refetch()
      }
      setClearAllOpen(false)
    } finally {
      setClearAllPending(false)
    }
  }

  function onScroll(e: UIEvent<HTMLDivElement>) {
    if (!activeQuery.hasNextPage || activeQuery.isFetchingNextPage) return
    const el = e.currentTarget
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 200) {
      void activeQuery.fetchNextPage()
    }
  }

  const allRows: HistoryRow[] =
    mode === 'backend'
      ? (backendQuery.data?.pages.flatMap((page) => page.items) ?? []).map((entry) => ({
          id: entry.id,
          connectionId: entry.connection_id,
          sqlText: entry.sql_text,
          executedAt: entry.executed_at,
        }))
      : (localQuery.data?.pages.flatMap((page) => page.items) ?? []).map((entry) => ({
          id: entry.id,
          connectionId: entry.connectionId,
          sqlText: entry.sqlText,
          executedAt: entry.executedAt,
        }))

  const rows = allRows.filter((row) => !pendingDeleteIds.has(row.id))
  const listItems = buildHistoryListItems(rows)

  const rowVirtualizer = useVirtualizer({
    count: listItems.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => {
      const item = listItems[index]
      return item?.type === 'divider' ? DIVIDER_ROW_HEIGHT : DATA_ROW_HEIGHT
    },
    overscan: 8,
    getItemKey: (index) => {
      const item = listItems[index]
      if (!item) return index
      return item.type === 'divider' ? item.key : item.row.id
    },
  })
  const virtualItems = rowVirtualizer.getVirtualItems()

  if (mode === 'off') {
    return (
      <SidebarPane
        title="History"
        icon="history"
        scroll={false}
        maximized={isMaximized}
        onMaximizedChange={onMaximize}
        onClose={onClose}
      >
        <IdeEmptyState
          icon="history"
          title="Query history is turned off"
          description="Queries won't be recorded while it is disabled."
        />
      </SidebarPane>
    )
  }

  const clearAllConnectionName = connections.data?.items.find(
    (c) => c.id === filterConnectionId,
  )?.name

  return (
    <SidebarPane
      title="History"
      icon="history"
      scroll={false}
      maximized={isMaximized}
      onMaximizedChange={onMaximize}
      onClose={onClose}
      headerContent={
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <HistoryConnectionSelector
            connections={connections.data?.items ?? []}
            environments={environments.data?.items ?? []}
            isLoading={connections.isLoading}
            value={connectionFilter}
            activeHintConnectionId={activeConnectionId}
            onChange={setConnectionFilter}
          />
          <SearchInput
            value={searchText}
            onValueChange={setSearchText}
            onClear={clearSearch}
            placeholder="Search query history…"
            className="min-w-0 flex-1"
            size="sm"
            variant="muted"
          />
        </div>
      }
      actions={
        <Tip
          label={
            filterConnectionId
              ? 'Clear all history for this connection'
              : 'Select a connection to clear its history'
          }
        >
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Clear all history"
            disabled={!filterConnectionId}
            onClick={() => setClearAllOpen(true)}
          >
            <Icon name="delete-02" size={14} />
          </Button>
        </Tip>
      }
    >
      <div className="@container flex h-full min-h-0 flex-col">
        <div
          ref={scrollRef}
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
            <HistoryEmptyState filtered={Boolean(debouncedQuery)} />
          ) : (
            <>
              <Table className="table-fixed">
                <HistoryColumnWidths />
                <HistoryTableHeader />
                <TableBody>
                  {virtualItems.length > 0 && (
                    <tr aria-hidden>
                      <td
                        colSpan={HISTORY_COLUMN_COUNT}
                        style={{ height: virtualItems[0]!.start }}
                      />
                    </tr>
                  )}
                  {virtualItems.map((vr) => {
                    const item = listItems[vr.index]
                    if (!item) return null

                    if (item.type === 'divider') {
                      return (
                        <HistoryDateDividerRow
                          key={item.key}
                          label={item.label}
                          dataIndex={vr.index}
                          measureRef={rowVirtualizer.measureElement}
                        />
                      )
                    }

                    const row = item.row
                    const connection = connections.data?.items.find(
                      (c) => c.id === row.connectionId,
                    )
                    const favoriteId = favoriteIdByKey.get(
                      favoriteKey(row.connectionId, row.sqlText),
                    )
                    const isFavorited = favoriteId !== undefined
                    return (
                      <HistoryRowItem
                        key={row.id}
                        dataIndex={vr.index}
                        measureRef={rowVirtualizer.measureElement}
                        row={row}
                        index={item.rowNumber}
                        connectionName={connection?.name}
                        driver={connection?.driver}
                        isFavorited={isFavorited}
                        canDelete={mode === 'backend'}
                        expanded={expandedIds.has(row.id)}
                        onToggleExpand={() => toggleExpand(row.id)}
                        onToggleFavorite={() => {
                          if (favoriteId !== undefined) {
                            void handleRemoveFavorite(favoriteId)
                          } else {
                            setFavoriteRow(row)
                          }
                        }}
                        onCopy={() => handleCopy(row.sqlText)}
                        onInsertAtCursor={() => handleInsertAtCursor(row)}
                        onDelete={() => scheduleDelete(row)}
                      />
                    )
                  })}
                  {virtualItems.length > 0 && (
                    <tr aria-hidden>
                      <td
                        colSpan={HISTORY_COLUMN_COUNT}
                        style={{
                          height:
                            rowVirtualizer.getTotalSize() -
                            virtualItems[virtualItems.length - 1]!.end,
                        }}
                      />
                    </tr>
                  )}
                </TableBody>
              </Table>
              {activeQuery.isFetchingNextPage && (
                <div className="flex items-center justify-center gap-1.5 py-2 text-[10px] text-muted-foreground">
                  <Icon name="loading-03" size={12} className="animate-spin" />
                  Loading more…
                </div>
              )}
            </>
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

      <AlertDialog
        open={clearAllOpen}
        onOpenChange={(next) => {
          if (next || !clearAllPending) setClearAllOpen(next)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Clear history for{' '}
              <span className="font-mono">{clearAllConnectionName ?? 'this connection'}</span>?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes every recorded query for this connection. This can't be
              undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel variant="ghost" disabled={clearAllPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={clearAllPending}
              onClick={handleClearAll}
            >
              {clearAllPending ? 'Clearing…' : 'Clear all'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SidebarPane>
  )
}
