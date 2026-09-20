import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { toast } from 'sonner'
import {
  allOrgWorkspaceConnectionsQueryOptions,
  orgRuntimeSettingsQueryOptions,
} from '#/lib/api/query'
import { orgWorkspaceQueryFavoritesQueryOptions } from '#/lib/api/queries/query-favorites'
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
import type { BottomPanelTabProps } from './bottomPanels'
import { insertAtCursor } from './insertAtCursor'
import { listLocalFavorites, type LocalFavorite } from './localQueryStore'
import { ReadOnlySqlView } from './object-detail/ReadOnlySqlView'
import { highlightSqlStatic } from './object-detail/staticSqlHighlight'
import { formatExactTime } from './relativeTime'
import { SidebarPane } from './SidebarPane'
import { Tip } from './schema-diagram/Tip'
import { isExpandableSql, flattenSql } from './sqlPreview'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useFavoritesMutations } from './useFavoritesMutations'
import { useIde, activeTabId as selectActiveTabId } from './useIdeStore'
import { IdeEmptyState } from './IdeEmptyState'

const DATA_ROW_HEIGHT = 36
const FAVORITES_COLUMN_COUNT = 6
const DELETE_UNDO_WINDOW_MS = 5000

type FavoriteRow = {
  id: number | string
  connectionId: number | null
  name: string
  sqlText: string
  createdAt: string
}

function FavoritesEmptyState({ filtered = false }: { filtered?: boolean }) {
  return (
    <IdeEmptyState
      icon={filtered ? 'search-01' : 'star'}
      title={filtered ? 'No matching favorites' : 'No saved favorites yet'}
      description={
        filtered ? 'Try a different search term.' : 'Save a query as a favorite to see it here.'
      }
    />
  )
}

function FavoritesColumnWidths() {
  return (
    <colgroup>
      <col className="w-9" />
      <col className="w-36" />
      <col className="w-36" />
      <col />
      <col className="w-48" />
      <col className="w-28" />
    </colgroup>
  )
}

function FavoritesTableHeader() {
  return (
    <TableHeader className="sticky top-0 z-10 hidden bg-muted/20 @lg:table-header-group">
      <TableRow className="hover:bg-transparent">
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          #
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Name
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Connection
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Query
        </TableHead>
        <TableHead className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Saved at
        </TableHead>
        <TableHead className="text-right text-[10px] uppercase tracking-wide text-muted-foreground/70">
          Actions
        </TableHead>
      </TableRow>
    </TableHeader>
  )
}

type FavoriteRowItemProps = {
  row: FavoriteRow
  index: number
  dataIndex: number
  measureRef: (el: HTMLTableRowElement | null) => void
  connectionName: string | undefined
  driver: string | undefined
  expanded: boolean
  onToggleExpand: () => void
  onCopy: () => void
  onInsertAtCursor: () => void
  onDelete: () => void
}

function FavoriteRowItem({
  row,
  index,
  dataIndex,
  measureRef,
  connectionName,
  driver,
  expanded,
  onToggleExpand,
  onCopy,
  onInsertAtCursor,
  onDelete,
}: FavoriteRowItemProps) {
  const expandable = isExpandableSql(row.sqlText)
  const cellAlign = expanded ? 'align-top' : 'align-middle'

  return (
    <TableRow
      data-testid="favorite-row"
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
        <span className="block min-w-0 truncate text-[10px] text-muted-foreground">{row.name}</span>
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
          {formatExactTime(row.createdAt)}
        </span>
      </TableCell>

      <TableCell className={`${cellAlign} text-right`}>
        <div className="flex shrink-0 items-center justify-end gap-1">
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
          <Tip label="Delete favorite">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Delete favorite"
              onClick={onDelete}
            >
              <Icon name="delete-01" size={12} />
            </Button>
          </Tip>
        </div>
      </TableCell>
    </TableRow>
  )
}

export function FavoritesPanel({ orgSlug, workspace }: BottomPanelTabProps) {
  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const viewRegistry = useEditorViewRegistry()
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText()

  const [expandedIds, setExpandedIds] = useState<Set<number | string>>(new Set())
  const [pendingDeleteIds, setPendingDeleteIds] = useState<Set<number | string>>(new Set())
  const deleteTimers = useRef(new Map<number | string, ReturnType<typeof setTimeout>>())
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(
    () => () => {
      deleteTimers.current.forEach((timer) => clearTimeout(timer))
    },
    [],
  )

  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const mode = runtimeSettings.data?.effective.query_favorites_mode ?? 'backend'
  const mutations = useFavoritesMutations(orgSlug, workspace.id)
  const connections = useQuery(allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id))

  const backendQuery = useQuery({
    ...orgWorkspaceQueryFavoritesQueryOptions(orgSlug, workspace.id, debouncedQuery || undefined),
    enabled: mode === 'backend',
  })

  const [localFavorites, setLocalFavorites] = useState<LocalFavorite[]>([])
  useEffect(() => {
    if (mode !== 'local') {
      setLocalFavorites([])
      return
    }
    let cancelled = false
    void listLocalFavorites(workspace.id).then((favorites) => {
      if (!cancelled) setLocalFavorites(favorites)
    })
    return () => {
      cancelled = true
    }
  }, [mode, workspace.id])

  function handleCopy(sqlText: string) {
    copyWithToast(sqlText, 'Query copied')
  }

  function handleInsert(sqlText: string) {
    if (!activeTabId || !activeGroupId) return
    const view = viewRegistry.get(`${activeGroupId}:${activeTabId}`)
    if (!view) return
    insertAtCursor(view, sqlText)
  }

  function toggleExpand(id: number | string) {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function scheduleDelete(row: FavoriteRow) {
    setPendingDeleteIds((prev) => new Set(prev).add(row.id))
    const timer = setTimeout(() => {
      deleteTimers.current.delete(row.id)
      void mutations.remove(row.id).then(() => {
        if (mode === 'backend') void backendQuery.refetch()
      })
    }, DELETE_UNDO_WINDOW_MS)
    deleteTimers.current.set(row.id, timer)
    toast('Favorite deleted', {
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

  const allRows: FavoriteRow[] =
    mode === 'backend'
      ? (backendQuery.data ?? []).map((fav) => ({
          id: fav.id,
          connectionId: fav.connection_id,
          name: fav.name,
          sqlText: fav.sql_text,
          createdAt: fav.created_at,
        }))
      : localFavorites.map((fav) => ({
          id: fav.id,
          connectionId: fav.connectionId,
          name: fav.name,
          sqlText: fav.sqlText,
          createdAt: fav.createdAt,
        }))

  const searchedRows =
    mode === 'local' && debouncedQuery
      ? allRows.filter((row) => {
          const term = debouncedQuery.toLowerCase()
          return row.name.toLowerCase().includes(term) || row.sqlText.toLowerCase().includes(term)
        })
      : allRows

  const rows = searchedRows.filter((row) => !pendingDeleteIds.has(row.id))

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => DATA_ROW_HEIGHT,
    overscan: 8,
    getItemKey: (index) => rows[index]?.id ?? index,
  })
  const virtualItems = rowVirtualizer.getVirtualItems()

  if (mode === 'off') {
    return (
      <SidebarPane title="Favorites" icon="star" scroll={false}>
        <IdeEmptyState
          icon="star"
          title="Query favorites are turned off"
          description="Saved queries aren't available while it is disabled."
        />
      </SidebarPane>
    )
  }

  return (
    <SidebarPane
      title="Favorites"
      icon="star"
      scroll={false}
      headerContent={
        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search favorites…"
          className="w-full"
          size="sm"
          variant="muted"
        />
      }
    >
      <div className="@container flex h-full min-h-0 flex-col">
        <div
          ref={scrollRef}
          className="min-h-0 flex-1 overflow-y-auto"
          data-testid="favorites-scroll"
        >
          {rows.length === 0 ? (
            <FavoritesEmptyState filtered={Boolean(debouncedQuery)} />
          ) : (
            <Table className="table-fixed">
              <FavoritesColumnWidths />
              <FavoritesTableHeader />
              <TableBody>
                {virtualItems.length > 0 && (
                  <tr aria-hidden>
                    <td
                      colSpan={FAVORITES_COLUMN_COUNT}
                      style={{ height: virtualItems[0]!.start }}
                    />
                  </tr>
                )}
                {virtualItems.map((vr) => {
                  const row = rows[vr.index]
                  if (!row) return null
                  const connection = connections.data?.items.find((c) => c.id === row.connectionId)
                  return (
                    <FavoriteRowItem
                      key={row.id}
                      dataIndex={vr.index}
                      measureRef={rowVirtualizer.measureElement}
                      row={row}
                      index={vr.index + 1}
                      connectionName={connection?.name}
                      driver={connection?.driver}
                      expanded={expandedIds.has(row.id)}
                      onToggleExpand={() => toggleExpand(row.id)}
                      onCopy={() => handleCopy(row.sqlText)}
                      onInsertAtCursor={() => handleInsert(row.sqlText)}
                      onDelete={() => scheduleDelete(row)}
                    />
                  )
                })}
                {virtualItems.length > 0 && (
                  <tr aria-hidden>
                    <td
                      colSpan={FAVORITES_COLUMN_COUNT}
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
          )}
        </div>
      </div>
    </SidebarPane>
  )
}
