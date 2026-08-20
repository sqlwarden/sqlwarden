import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { orgRuntimeSettingsQueryOptions } from '#/lib/api/query'
import { orgWorkspaceQueryFavoritesQueryOptions } from '#/lib/api/queries/query-favorites'
import { SearchInput } from '#/components/SearchInput'
import { Button } from '#/components/ui/button'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'
import { Icon } from '#/lib/icons'
import { copyWithToast } from './contextMenus/clipboard'
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
  name: string
  sqlText: string
  createdAt: string
}

export function FavoritesPanel({ orgSlug, workspace }: IdeSidebarPanelProps) {
  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const viewRegistry = useEditorViewRegistry()
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText()

  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const mode = runtimeSettings.data?.effective.query_favorites_mode ?? 'backend'
  const mutations = useFavoritesMutations(orgSlug, workspace.id)

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
  }

  if (mode === 'off') {
    return (
      <SidebarPane title="Favorites" icon="star">
        <div className="px-3 py-4 text-center text-xs text-muted-foreground">
          Query favorites are turned off for this organization.
        </div>
      </SidebarPane>
    )
  }

  const allRows: FavoriteRow[] =
    mode === 'backend'
      ? (backendQuery.data ?? []).map((fav) => ({
          id: fav.id,
          name: fav.name,
          sqlText: fav.sql_text,
          createdAt: fav.created_at,
        }))
      : localFavorites.map((fav) => ({
          id: fav.id,
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

  return (
    <SidebarPane title="Favorites" icon="star" scroll={false}>
      <div className="flex h-full min-h-0 flex-col">
        <div className="shrink-0 border-b border-border p-2">
          <SearchInput
            value={searchText}
            onValueChange={setSearchText}
            onClear={clearSearch}
            placeholder="Search favorites…"
            className="w-full"
          />
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
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
            <div className="flex flex-col gap-2 p-2">
              {rows.map((row) => (
                <div
                  key={row.id}
                  data-testid="favorite-row"
                  className="flex flex-col gap-2 rounded-lg border border-border/60 bg-card/40 p-2.5 transition-colors hover:border-border hover:bg-muted/20"
                >
                  <div className="flex items-center gap-1.5">
                    <span className="min-w-0 flex-1 truncate text-xs font-medium text-foreground">
                      {row.name}
                    </span>
                  </div>

                  <div className="max-h-16 overflow-y-auto rounded-md bg-muted/40 px-2 py-1.5">
                    <code className="block whitespace-pre-wrap break-words font-mono text-xs leading-snug text-foreground">
                      {row.sqlText}
                    </code>
                  </div>

                  <div className="flex items-center justify-between gap-1">
                    <Tip label={formatExactTime(row.createdAt)}>
                      <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
                        {formatRelativeTime(row.createdAt)}
                      </span>
                    </Tip>
                    <div className="flex items-center gap-1">
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
                      <Tip label="Delete favorite">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label="Delete favorite"
                          onClick={() => void handleDelete(row.id)}
                        >
                          <Icon name="delete-01" size={12} />
                        </Button>
                      </Tip>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </SidebarPane>
  )
}
