import { lazy, Suspense, useMemo, useState } from 'react'
import * as Y from 'yjs'
import { toast } from 'sonner'
import { useQuery } from '@tanstack/react-query'
import { cn } from '#/lib/utils'
import { formatBytesValue } from '#/lib/units'
import type { Workspace } from '#/lib/api/types'
import { downloadPrivateWorkspaceFile } from '#/lib/api/files'
import { allOrgWorkspaceConnectionsQueryOptions } from '#/lib/api/query'
import { Button } from '#/components/ui/button'
import { Skeleton } from '#/components/ui/skeleton'
import { Icon } from '#/lib/icons'
import { useIde } from './useIdeStore'
import type { GroupNode } from './ideLayout'
import { IdeTabBar } from './IdeTabBar'
import { SqlEditor } from './SqlEditor'
import { useYDocRegistry } from './useYDocRegistry'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useFileContent } from './useFileContent'
import { ObjectDetailView } from './object-detail/ObjectDetailView'
import { CsvViewer } from './csv/CsvViewer'
import { isCsvFileTab, isCsvFileTooLarge, MAX_BROWSER_CSV_BYTES } from './csv/csvFile'
import { saveBlobAs } from './saveFile'
import { isSqlEditorTab, splitSqlStatements } from './sqlStatements'
import { formatEditorSql, sqlFormatterForDriver } from './sqlFormatting'
import { useToolbarQueryAction } from './useToolbarQueryAction'
import { SaveFavoriteDialog } from './SaveFavoriteDialog'

const SchemaDiagramView = lazy(() =>
  import('./schema-diagram/SchemaDiagramView').then((module) => ({
    default: module.SchemaDiagramView,
  })),
)

type EditorGroupProps = {
  orgSlug: string
  workspace: Workspace
  group: GroupNode
  focused: boolean
  /** Whether to render the focus ring (only meaningful when split into multiple groups). */
  showFocus?: boolean
  onCursorChange?: (line: number, col: number, sel: number) => void
}

export function EditorGroup({
  orgSlug,
  workspace,
  group,
  focused,
  showFocus,
  onCursorChange,
}: EditorGroupProps) {
  const registry = useYDocRegistry()
  const viewRegistry = useEditorViewRegistry()
  const tabs = useIde((s) => s.tabs)
  const focusGroup = useIde((s) => s.focusGroup)
  const updateTabEtag = useIde((s) => s.updateTabEtag)
  const [downloadingFileId, setDownloadingFileId] = useState<number | null>(null)
  const [saveFavoriteOpen, setSaveFavoriteOpen] = useState(false)
  const [saveFavoriteSql, setSaveFavoriteSql] = useState('')

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === group.activeTabId),
    [tabs, group.activeTabId],
  )
  const sessionId = useIde((s) =>
    activeTab?.connectionId === undefined ? undefined : s.sessions[activeTab.connectionId],
  )

  const isObject = activeTab?.kind === 'object'
  const isDiagram = activeTab?.kind === 'diagram'
  const isCsv = isCsvFileTab(activeTab)
  const isCsvTooLarge = isCsvFileTooLarge(activeTab)
  const isSqlTab = isSqlEditorTab(activeTab)

  // Same query key IdeToolbar uses for the workspace-wide connection list —
  // React Query dedupes the request, this just adds a subscriber.
  const connections = useQuery(allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id))
  const connItems = connections.data?.items ?? []
  const activeConnection = connItems.find((c) => c.id === activeTab?.connectionId)

  // Scoped to this group's own tab, with the Ctrl/Cmd+Enter shortcut left to
  // the toolbar's instance — this one only backs the editor's context menu.
  const queryAction = useToolbarQueryAction({
    orgSlug,
    workspace,
    activeTab,
    activeGroupId: group.id,
    activeConnection,
    hasConnections: connItems.length > 0,
    bindShortcut: false,
  })

  function handleFormat() {
    if (!activeTab) return
    const view = viewRegistry.get(`${group.id}:${activeTab.id}`)
    if (!view) return
    try {
      formatEditorSql(view, sqlFormatterForDriver(activeTab.driver))
    } catch {
      toast.error('Could not format SQL.', {
        description: 'The query may contain unsupported or incomplete syntax.',
      })
    }
  }

  function handleRunAll() {
    const sqls = splitSqlStatements(queryAction.resolveDocumentText())
    if (sqls.length === 0) return
    void queryAction.runAll(sqls)
  }

  function handleSaveFavoriteClick() {
    const sql = queryAction.resolveSql()
    if (!sql) return
    setSaveFavoriteSql(sql)
    setSaveFavoriteOpen(true)
  }

  const { isLoading, isError, retry } = useFileContent({
    orgSlug,
    workspaceId: workspace.id,
    tab: activeTab,
    updateTabEtag,
    enabled: !isCsvTooLarge,
  })

  async function downloadCsv() {
    if (activeTab?.kind !== 'file' || activeTab.fileId === undefined) return
    setDownloadingFileId(activeTab.fileId)
    try {
      const blob = await downloadPrivateWorkspaceFile(orgSlug, workspace.id, activeTab.fileId)
      saveBlobAs(activeTab.title, blob)
    } catch {
      toast.error('Failed to download CSV.')
    } finally {
      setDownloadingFileId(null)
    }
  }

  // Populate the Y.Doc synchronously in render so SqlEditor mounts with content
  // (React runs child effects before parent effects, so deferring is too late).
  // Object and diagram tabs are not editors and never get a Y.Doc.
  let doc: Y.Doc | undefined
  if (activeTab && !isObject && !isDiagram && !isCsvTooLarge) {
    const initState = activeTab.ySnapshot ?? activeTab.yState
    const initialContent = !initState && activeTab.kind !== 'file' ? activeTab.content : undefined
    doc = registry.getOrCreate(activeTab.id, initialContent)
    if (initState && doc.getText('content').length === 0) {
      Y.applyUpdate(doc, new Uint8Array(initState), 'init')
    }
  }

  return (
    <div
      className={cn(
        'flex h-full min-h-0 flex-col',
        showFocus && focused && 'ring-1 ring-inset ring-primary/25',
      )}
      onMouseDownCapture={() => focusGroup(workspace.id, group.id)}
    >
      <IdeTabBar
        orgSlug={orgSlug}
        workspace={workspace}
        group={group}
        focused={focused}
        onFocus={() => focusGroup(workspace.id, group.id)}
      />
      <div className="min-h-0 flex-1 border-t border-border bg-card">
        {activeTab && isDiagram ? (
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
                Loading diagram...
              </div>
            }
          >
            <SchemaDiagramView
              key={`${group.id}:${activeTab.id}`}
              orgSlug={orgSlug}
              workspace={workspace}
              tab={activeTab}
            />
          </Suspense>
        ) : activeTab && isObject ? (
          <ObjectDetailView
            key={`${group.id}:${activeTab.id}`}
            orgSlug={orgSlug}
            workspace={workspace}
            tab={activeTab}
          />
        ) : activeTab && isCsvTooLarge ? (
          <CsvTooLargeState
            filename={activeTab.title}
            sizeBytes={activeTab.fileSizeBytes!}
            downloading={downloadingFileId === activeTab.fileId}
            onDownload={() => void downloadCsv()}
          />
        ) : activeTab && doc ? (
          isLoading ? (
            isCsv ? (
              <CsvViewerSkeleton />
            ) : (
              <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
                Loading…
              </div>
            )
          ) : isError ? (
            <div className="flex h-full flex-col items-center justify-center gap-2.5 text-xs">
              <span className="text-destructive">Failed to load file content.</span>
              <button
                type="button"
                onClick={retry}
                className="rounded-md border border-border px-2.5 py-1 text-xs text-foreground transition-colors hover:bg-accent"
              >
                Retry
              </button>
            </div>
          ) : isCsv ? (
            <CsvViewer key={`${group.id}:${activeTab.id}`} doc={doc} className="h-full" />
          ) : (
            <SqlEditor
              key={`${group.id}:${activeTab.id}`}
              tabId={activeTab.id}
              groupId={group.id}
              doc={doc}
              className="h-full"
              onCursorChange={focused ? onCursorChange : undefined}
              driver={activeTab.driver}
              completion={{
                orgSlug,
                workspaceId: workspace.id,
                connectionId: activeTab.connectionId,
                driver: activeTab.driver,
                sessionId,
              }}
              contextMenu={{
                isSqlTab,
                canRun: Boolean(activeConnection) && !queryAction.isRunning,
                onRunStatement: () => void queryAction.run(),
                onRunAll: handleRunAll,
                onFormat: handleFormat,
                onSaveFavorite: handleSaveFavoriteClick,
              }}
            />
          )
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            No editor in this group
          </div>
        )}
      </div>

      {saveFavoriteOpen && (
        <SaveFavoriteDialog
          open={saveFavoriteOpen}
          onOpenChange={setSaveFavoriteOpen}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          sqlText={saveFavoriteSql}
          connectionId={activeConnection?.id ?? null}
        />
      )}
    </div>
  )
}

function CsvTooLargeState({
  filename,
  sizeBytes,
  downloading,
  onDownload,
}: {
  filename: string
  sizeBytes: number
  downloading: boolean
  onDownload: () => void
}) {
  return (
    <div className="flex h-full items-center justify-center p-8 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex size-10 items-center justify-center rounded-lg border border-border bg-muted/50">
          <Icon name="file-01" size={17} className="text-muted-foreground" />
        </div>
        <div className="flex flex-col gap-1">
          <div className="text-sm font-medium text-foreground">CSV is too large to preview</div>
          <div className="text-xs text-muted-foreground">
            <span className="font-medium text-foreground">{filename}</span> is{' '}
            {formatBytesValue(sizeBytes)}. Browser previews are limited to{' '}
            {formatBytesValue(MAX_BROWSER_CSV_BYTES)} to keep the IDE responsive.
          </div>
        </div>
        <Button type="button" size="sm" onClick={onDownload} disabled={downloading}>
          <Icon
            name={downloading ? 'loading-03' : 'download-01'}
            size={13}
            className={downloading ? 'animate-spin' : undefined}
            data-icon="inline-start"
          />
          {downloading ? 'Downloading…' : 'Download CSV'}
        </Button>
      </div>
    </div>
  )
}

/** Mirrors CsvViewer's toolbar/grid chrome while content is loading. */
function CsvViewerSkeleton() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border bg-background px-2">
        <Skeleton className="h-6 w-56 max-w-[55%]" />
      </div>
      <div className="min-h-0 flex-1 overflow-hidden" aria-label="Loading CSV">
        <div className="flex h-7 border-b border-border bg-muted/50">
          <Skeleton className="m-1 h-5 w-10 rounded-sm" />
          <Skeleton className="m-1 h-5 w-36 rounded-sm" />
          <Skeleton className="m-1 h-5 w-36 rounded-sm" />
        </div>
        {Array.from({ length: 7 }, (_, index) => (
          <div key={index} className="flex h-7 items-center gap-3 border-b border-border px-2">
            <Skeleton className="h-3 w-7 rounded-sm" />
            <Skeleton className="h-3 w-28 rounded-sm" />
            <Skeleton className="h-3 w-32 rounded-sm" />
          </div>
        ))}
      </div>
    </div>
  )
}
