import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  allOrgWorkspaceConnectionsQueryOptions,
  orgRuntimeSettingsQueryOptions,
} from '#/lib/api/query'
import { orgWorkspaceQueryFavoritesQueryOptions } from '#/lib/api/queries/query-favorites'
import { SearchInput } from '#/components/SearchInput'
import { Button } from '#/components/ui/button'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { Icon } from '#/lib/icons'
import { copyWithToast } from './contextMenus/clipboard'
import { DriverBadge } from './DriverBadge'
import { FavoriteQueryDialog } from './FavoriteQueryDialog'
import type { IdeSidebarPanelProps } from './ideActivities'
import { insertAtCursor } from './insertAtCursor'
import { listLocalFavorites, type LocalFavorite } from './localQueryStore'
import { formatExactTime, formatRelativeTime } from './relativeTime'
import { SidebarPane } from './SidebarPane'
import { Tip } from './schema-diagram/Tip'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useFavoritesMutations } from './useFavoritesMutations'
import { useIde, activeTabId as selectActiveTabId } from './useIdeStore'

type FavoriteRow = {
  id: number | string
  connectionId: number | null
  name: string
  sqlText: string
  createdAt: string
}

// Rows render at a fixed height (name/SQL lines are single-line truncated, so
// content height doesn't vary) so the virtualizer can size items without
// measuring the DOM.
const ROW_HEIGHT = 84

type FavoriteRowItemProps = {
  row: FavoriteRow
  connectionName: string | undefined
  driver: string | undefined
  onView: () => void
  onCopy: () => void
  onInsert: () => void
  onDelete: () => void
}

function FavoriteRowItem({
  row,
  connectionName,
  driver,
  onView,
  onCopy,
  onInsert,
  onDelete,
}: FavoriteRowItemProps) {
  return (
    <div
      data-testid="favorite-row"
      className="flex h-full flex-col gap-1 border-l-2 border-transparent py-1.5 pl-2.5 pr-2 transition-colors hover:bg-muted/20"
    >
      <div className="flex shrink-0 items-center gap-1.5">
        <span className="min-w-0 flex-1 truncate text-[10px] font-normal text-muted-foreground">
          {row.name}
        </span>
      </div>

      <button
        type="button"
        onClick={onView}
        className="block shrink-0 truncate rounded-sm text-left font-mono text-xs leading-snug text-foreground hover:text-foreground/80"
      >
        {row.sqlText}
      </button>

      <div className="flex shrink-0 items-center justify-between gap-1">
        <div className="flex min-w-0 items-center gap-1.5">
          <Tip label={formatExactTime(row.createdAt)}>
            <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
              {formatRelativeTime(row.createdAt)}
            </span>
          </Tip>
          {connectionName && (
            <span className="flex min-w-0 items-center gap-1 truncate text-[10px] text-muted-foreground">
              {driver && <DriverBadge driver={driver} size="sm" className="size-3" />}
              <span className="truncate">{connectionName}</span>
            </span>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1">
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
              onClick={onInsert}
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
      </div>
    </div>
  )
}

export function FavoritesPanel({ orgSlug, workspace }: IdeSidebarPanelProps) {
  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const viewRegistry = useEditorViewRegistry()
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText()

  const [viewRow, setViewRow] = useState<FavoriteRow | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

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

  async function handleDelete(id: number | string) {
    await mutations.remove(id)
    if (mode === 'backend') await backendQuery.refetch()
    setViewRow((current) => (current?.id === id ? null : current))
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

  const rows =
    mode === 'local' && debouncedQuery
      ? allRows.filter((row) => {
          const term = debouncedQuery.toLowerCase()
          return row.name.toLowerCase().includes(term) || row.sqlText.toLowerCase().includes(term)
        })
      : allRows

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  })
  const virtualItems = rowVirtualizer.getVirtualItems()

  if (mode === 'off') {
    return (
      <SidebarPane title="Favorites" icon="star">
        <div className="px-3 py-4 text-center text-xs text-muted-foreground">
          Query favorites are turned off for this organization.
        </div>
      </SidebarPane>
    )
  }

  const viewRowConnection = connections.data?.items.find((c) => c.id === viewRow?.connectionId)

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
      <div className="flex h-full min-h-0 flex-col">
        <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto">
          {rows.length === 0 ? (
            <div className="px-3 py-4 text-center text-xs text-muted-foreground">
              {debouncedQuery ? (
                <>
                  <p className="font-medium text-foreground">No matching favorites</p>
                  <p className="mt-0.5">Try a different search term.</p>
                </>
              ) : (
                <>
                  <p className="font-medium text-foreground">No saved favorites yet</p>
                  <p className="mt-0.5">Save a query from the toolbar to see it here.</p>
                </>
              )}
            </div>
          ) : (
            <div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}>
              {virtualItems.map((vr) => {
                const row = rows[vr.index]
                if (!row) return null
                const connection = connections.data?.items.find((c) => c.id === row.connectionId)
                return (
                  <div
                    key={row.id}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      height: vr.size,
                      transform: `translateY(${vr.start}px)`,
                    }}
                  >
                    <FavoriteRowItem
                      row={row}
                      connectionName={connection?.name}
                      driver={connection?.driver}
                      onView={() => setViewRow(row)}
                      onCopy={() => handleCopy(row.sqlText)}
                      onInsert={() => handleInsert(row.sqlText)}
                      onDelete={() => void handleDelete(row.id)}
                    />
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>

      <FavoriteQueryDialog
        row={viewRow}
        connectionName={viewRowConnection?.name}
        driver={viewRowConnection?.driver}
        onOpenChange={(open) => {
          if (!open) setViewRow(null)
        }}
        onCopy={() => viewRow && handleCopy(viewRow.sqlText)}
        onInsert={() => viewRow && handleInsert(viewRow.sqlText)}
        onDelete={() => viewRow && void handleDelete(viewRow.id)}
      />
    </SidebarPane>
  )
}
