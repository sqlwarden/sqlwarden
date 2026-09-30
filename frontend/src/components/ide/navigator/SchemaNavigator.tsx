import { useDeferredValue, useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Icon } from '#/lib/icons'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, schemaTreeQueryOptions } from '#/lib/api/query'
import { queryKeys } from '#/lib/api/query-keys'
import type { Connection, NavigatorListing, ObjectRef, ScopePath, Workspace } from '#/lib/api/types'
import { hasAnyPermission, permission } from '#/lib/permissions'
import { newObjectTab } from '../object-detail/objectTab'
import { newDiagramTab, type DiagramTarget } from '../schema-diagram/diagramTab'
import { useEvictGoneSession } from '../sessionErrors'
import { dialectFor } from '../sqlDialect'
import { useIde } from '../useIdeStore'
import { useInsertIntoEditor } from '../useInsertIntoEditor'
import { useSchemaRefresh } from '../useSchemaRefresh'
import { NAVIGATOR_ROW_HEIGHT, NavigatorRowView } from './NavigatorRow'
import { columnFolderOf, type NavigatorActions } from './navigatorMenus'
import { SchemaEditDialogs, type EditTarget } from './SchemaEditDialogs'
import { useNavigatorRows } from './useNavigatorRows'
import { useScrollAnchor } from './useScrollAnchor'

function useDebouncedValue(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delayMs)
    return () => window.clearTimeout(timer)
  }, [value, delayMs])
  return debounced
}

export function SchemaNavigator({
  orgSlug,
  workspaceId,
  connectionId,
  driver,
  filter,
  onConnect,
  onFilteringChange,
}: {
  orgSlug: string
  workspaceId: number
  connectionId: number
  driver: string
  filter: string
  onConnect?: () => void
  /** Reports whether the debounced filter is still catching up to the typed value. */
  onFilteringChange?: (pending: boolean) => void
}) {
  const sessionId = useIde((s) => s.sessions[connectionId])
  const connStatus = useIde((s) => s.connectionStatus[connectionId])
  const openTab = useIde((s) => s.openTab)
  const insert = useInsertIntoEditor()
  const queryClient = useQueryClient()
  const dialect = dialectFor(driver)

  const treeQuery = useQuery(schemaTreeQueryOptions(orgSlug, workspaceId, connectionId))
  const permissionsQuery = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'connection', connectionId),
  )
  const refresh = useSchemaRefresh({ orgSlug, workspaceId, connectionId, sessionId })
  const [editTarget, setEditTarget] = useState<EditTarget | null>(null)

  const deferredFilter = useDeferredValue(useDebouncedValue(filter, 150))
  useEffect(() => {
    onFilteringChange?.(filter !== deferredFilter)
  }, [filter, deferredFilter, onFilteringChange])
  useEffect(() => () => onFilteringChange?.(false), [onFilteringChange])

  const tree = treeQuery.data
  const { rows, toggle, retry } = useNavigatorRows({
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
    tree,
    filter: deferredFilter,
  })
  useEvictGoneSession(connectionId, [treeQuery.error])

  const [listElement, setListElement] = useState<HTMLDivElement | null>(null)
  const { scrollElement, scrollMargin } = useScrollAnchor(listElement)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollElement,
    estimateSize: () => NAVIGATOR_ROW_HEIGHT,
    overscan: 12,
    scrollMargin,
    getItemKey: (index) => rows[index]?.key ?? index,
  })

  if (treeQuery.isPending) {
    return (
      <NavigatorMessage>
        <Icon name="loading-03" size={12} className="shrink-0 animate-spin" />
        Loading schema…
      </NavigatorMessage>
    )
  }
  if (treeQuery.isError || !tree) {
    if (isApiError(treeQuery.error) && treeQuery.error.status === 501) {
      return <NavigatorMessage>This driver doesn&apos;t support schema browsing.</NavigatorMessage>
    }
    return (
      <NavigatorMessage>
        Failed to load schema.
        <button
          type="button"
          className="underline hover:text-foreground"
          onClick={() => treeQuery.refetch()}
        >
          Retry
        </button>
      </NavigatorMessage>
    )
  }

  const connection = { id: connectionId, driver } as Connection
  const workspace = { id: workspaceId } as Workspace
  const actions: NavigatorActions = {
    tree,
    dialect,
    sessionId,
    canMutate: hasAnyPermission(permissionsQuery.data?.permissions, [
      permission.connExecute,
      permission.connDdl,
    ]),
    refresh: (path: ScopePath) => (sessionId ? refresh.mutate(path) : onConnect?.()),
    openObject: (ref: ObjectRef) => openTab(newObjectTab(connection, workspace, ref)),
    openDiagram: (target: DiagramTarget) => openTab(newDiagramTab(connection, workspace, target)),
    columnsOf: (path: ScopePath) => {
      const folder = columnFolderOf(tree, tree.nodes[path[path.length - 1]?.kind ?? ''])
      if (!folder) return []
      return (
        queryClient.getQueryData<NavigatorListing>(
          queryKeys.connectionSchemaNodes(orgSlug, workspaceId, connectionId, path, folder.kind),
        )?.items ?? []
      )
    },
    edit: {
      createTable: (scope) => setEditTarget({ type: 'create_table', scope }),
      dropScope: (scope, scopeKind) =>
        setEditTarget({ type: 'drop', target: { kind: 'scope', scope, scopeKind } }),
      dropObject: (ref) => setEditTarget({ type: 'drop', target: { kind: 'object', ref } }),
      editColumn: (ref, column) => setEditTarget({ type: 'column', ref, column }),
      renameColumn: (ref, columnName) => setEditTarget({ type: 'rename_column', ref, columnName }),
      dropColumn: (ref, columnName) =>
        setEditTarget({ type: 'drop', target: { kind: 'column', ref, columnName } }),
      createIndex: (ref) => setEditTarget({ type: 'index', ref }),
      dropIndex: (ref, indexName) =>
        setEditTarget({ type: 'drop', target: { kind: 'index', ref, indexName } }),
      generateStatement: (ref, operation) => setEditTarget({ type: 'generate', ref, operation }),
    },
  }
  const filtering = deferredFilter.trim() !== ''

  return (
    <>
      {!sessionId && (
        <NavigatorMessage className="border-b border-border bg-muted/40">
          {connStatus === 'connecting' ? (
            <>
              <Icon name="loading-03" size={12} className="shrink-0 animate-spin" />
              Connecting…
            </>
          ) : (
            <>
              Not connected. Showing cached objects.
              {onConnect && (
                <button
                  type="button"
                  className="font-medium text-primary hover:underline"
                  onClick={onConnect}
                >
                  Connect
                </button>
              )}
            </>
          )}
        </NavigatorMessage>
      )}
      {filtering && <NavigatorMessage>Filter matches loaded objects only.</NavigatorMessage>}
      {filtering && rows.length === 0 && <NavigatorMessage>No matches.</NavigatorMessage>}
      <div
        ref={setListElement}
        role="tree"
        aria-label="Schema"
        className="relative py-0.5"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const row = rows[virtualRow.index]
          return (
            <div
              key={virtualRow.key}
              className="absolute inset-x-0"
              style={{ top: virtualRow.start - scrollMargin, height: virtualRow.size }}
            >
              <NavigatorRowView
                row={row}
                actions={actions}
                insert={insert}
                onToggle={toggle}
                onRetry={retry}
                onConnect={onConnect}
              />
            </div>
          )
        })}
      </div>
      <SchemaEditDialogs
        orgSlug={orgSlug}
        workspaceId={workspaceId}
        connectionId={connectionId}
        sessionId={sessionId}
        tree={tree}
        editor={tree.editor}
        target={editTarget}
        onClose={() => setEditTarget(null)}
      />
    </>
  )
}

function NavigatorMessage({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={`flex items-center gap-1.5 px-2 py-1.5 text-xs text-muted-foreground ${className ?? ''}`}
    >
      {children}
    </div>
  )
}
