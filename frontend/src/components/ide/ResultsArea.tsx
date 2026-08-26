import { useState, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import { Icon, type AppIcon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '#/components/ui/resizable'
import { cn } from '#/lib/utils'
import type { Connection, ResultColumn, ResultValue, Workspace } from '#/lib/api/types'
import {
  useIde,
  activeTabId as selectActiveTabId,
  type EditorTab,
  type QueryResult,
  type ResultRun,
  type ResultsPanelMode,
} from './useIdeStore'
import { closeRunCursors } from './resultRunHistory'
import { visibleRuns, resolveSelectedRunId } from './resultRunFilter'
import { useContextMenuOpener } from '#/components/ui/context-menu'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { copyWithToast, rowToTsv, rowToJson, valuesToLines } from './contextMenus/clipboard'
import { buildCellMenu, buildRowMenu, buildColumnHeaderMenu } from './contextMenus/resultMenu'
import { buildResultTabMenu } from './contextMenus/resultTabMenu'
import { tabsToClose, type TabCloseScope } from './ideLayout'
import { nextCell } from './resultGridNav'
import { RUN_SHORTCUT } from './IdeToolbar'
import { Tip } from './schema-diagram/Tip'
import { DriverBadge } from './DriverBadge'
import { ExportButton } from './exports/ExportButton'
import { ViewQueryDialog } from './ViewQueryDialog'
import { columnTypeIcon, columnTypeIconColor } from './columnTypeIcon'
import { allOrgWorkspaceConnectionsQueryOptions } from '#/lib/api/query'
import {
  cellInRange,
  formatResultValue as formatValue,
  isRowInRange,
  type CellSelection,
} from './resultValues'
import { useResultCursorPaging } from './useResultCursorPaging'
import { useColumnResize } from './useColumnResize'

type ResultsAreaProps = {
  orgSlug: string
  workspace: Workspace
}

export function ResultsArea({ orgSlug, workspace }: ResultsAreaProps) {
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const activeTabId = useIde((s) =>
    s.activeWorkspaceId ? selectActiveTabId(s, s.activeWorkspaceId) : undefined,
  )
  const tabs = useIde((s) => s.tabs)
  const resultRuns = useIde((s) => s.resultRuns)
  const selectedRunId = useIde((s) => s.selectedRunId)
  const sharedSelectedRunId = useIde((s) => s.sharedSelectedRunId)
  const connectionSelectedRunId = useIde((s) => s.connectionSelectedRunId)
  const resultsPanelMode = useIde((s) => s.resultsPanelMode)
  const setResultsPanelMode = useIde((s) => s.setResultsPanelMode)
  const setSelectedRun = useIde((s) => s.setSelectedRun)
  const setSharedSelectedRun = useIde((s) => s.setSharedSelectedRun)
  const setConnectionSelectedRun = useIde((s) => s.setConnectionSelectedRun)
  const setSelectedIndexInRun = useIde((s) => s.setSelectedIndexInRun)
  const closeRunTab = useIde((s) => s.closeRunTab)
  const toggleRunPin = useIde((s) => s.toggleRunPin)

  // Same query key the toolbar uses, so this is a cache hit, not a new request.
  const connectionsQuery = useQuery(allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id))
  const connections = connectionsQuery.data?.items ?? []

  const activeConnectionId = tabs.find((t) => t.id === activeTabId)?.connectionId

  const runs: ResultRun[] = visibleRuns(
    resultsPanelMode,
    resultRuns,
    activeTabId,
    activeConnectionId,
  )
  const activeRunId = resolveSelectedRunId(
    resultsPanelMode,
    runs,
    activeTabId ? selectedRunId[activeTabId] : undefined,
    sharedSelectedRunId,
    activeConnectionId !== undefined ? connectionSelectedRunId[activeConnectionId] : undefined,
  )
  const activeRun = runs.find((r) => r.id === activeRunId)
  const resultList: QueryResult[] = activeRun?.results ?? []
  const rawSelectedIndex = activeRun?.selectedIndex ?? 0
  const selectedIndex = Math.min(Math.max(rawSelectedIndex, 0), Math.max(resultList.length - 1, 0))

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'results' ? null : 'results')
  }

  function handleSelectRun(runId: string) {
    if (resultsPanelMode === 'per-editor') {
      if (activeTabId) setSelectedRun(activeTabId, runId)
    } else if (resultsPanelMode === 'shared') {
      setSharedSelectedRun(runId)
    } else if (activeConnectionId !== undefined) {
      setConnectionSelectedRun(activeConnectionId, runId)
    }
  }

  // Runs shown outside 'per-editor' mode may originate from a tab other than
  // the focused one, so these act on the run's own tabId rather than assuming
  // the focused tab owns it.
  function handleCloseRun(runId: string) {
    const run = runs.find((r) => r.id === runId)
    if (!run) return
    closeRunTab(run.tabId, runId)
    const tab = tabs.find((t) => t.id === run.tabId)
    void closeRunCursors(orgSlug, workspace.id, tab?.connectionId, run)
  }

  function handleCloseRuns(runIds: string[]) {
    runIds.forEach(handleCloseRun)
  }

  function handleTogglePin(runId: string) {
    const run = runs.find((r) => r.id === runId)
    if (run) toggleRunPin(run.tabId, runId)
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-0">
      <div className="flex h-8 shrink-0 items-center bg-sidebar">
        <RunTabStrip
          runs={runs}
          connections={connections}
          tabs={tabs}
          mode={resultsPanelMode}
          activeRunId={activeRun?.id}
          onSelect={handleSelectRun}
          onClose={handleCloseRun}
          onCloseMany={handleCloseRuns}
          onTogglePin={handleTogglePin}
        />
        <div className="flex shrink-0 items-center gap-0.5 border-l border-border px-1">
          <ResultsPanelModeMenu mode={resultsPanelMode} onChange={setResultsPanelMode} />
          <Tip
            label={maximizedPane === 'results' ? 'Restore results panel' : 'Maximize results panel'}
          >
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Toggle results maximize"
              onClick={toggleMaximize}
            >
              <Icon name={maximizedPane === 'results' ? 'minimize' : 'maximize'} size={14} />
            </Button>
          </Tip>
          <Tip label="Hide results panel">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Hide results panel"
              onClick={() => setMaximizedPane('editor')}
            >
              <Icon name="cancel-01" size={14} />
            </Button>
          </Tip>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        <ResultsContent
          key={`${activeTabId ?? ''}-${activeRun?.id ?? ''}`}
          orgSlug={orgSlug}
          workspace={workspace}
          activeTabId={activeTabId}
          runId={activeRun?.id ?? ''}
          results={resultList}
          selectedIndex={selectedIndex}
          connections={connections}
          runConnectionId={activeRun?.connectionId}
          onSelectIndex={(index) =>
            activeRun && setSelectedIndexInRun(activeRun.tabId, activeRun.id, index)
          }
        />
      </div>
    </div>
  )
}

// ─── Result content switcher ────────────────────────────────────────────────

/** A result's own `connectionId` (only 'ok' results carry one) takes priority
 *  over the run's connectionId, so a statement keeps the connection it actually
 *  ran against even if reused across runs. */
function resolveResultConnection(
  connections: Connection[],
  result: QueryResult,
  runConnectionId: number | undefined,
): Connection | undefined {
  const id = (result.status === 'ok' ? result.connectionId : undefined) ?? runConnectionId
  return connections.find((c) => c.id === id)
}

function ResultsContent({
  orgSlug,
  workspace,
  activeTabId,
  runId,
  results,
  selectedIndex,
  connections,
  runConnectionId,
  onSelectIndex,
}: {
  orgSlug: string
  workspace: Workspace
  activeTabId?: string
  runId: string
  results: QueryResult[]
  selectedIndex: number
  connections: Connection[]
  runConnectionId: number | undefined
  onSelectIndex: (index: number) => void
}) {
  if (results.length === 0) return <EmptyState />

  if (results.length === 1) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <ResultEntry
          orgSlug={orgSlug}
          workspace={workspace}
          activeTabId={activeTabId}
          runId={runId}
          result={results[0]}
          index={0}
          connection={resolveResultConnection(connections, results[0], runConnectionId)}
        />
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-0">
      <ResultsSidebar results={results} selectedIndex={selectedIndex} onSelect={onSelectIndex} />
      <div className="min-w-0 flex-1">
        <ResultEntry
          orgSlug={orgSlug}
          workspace={workspace}
          activeTabId={activeTabId}
          runId={runId}
          result={results[selectedIndex]}
          index={selectedIndex}
          connection={resolveResultConnection(connections, results[selectedIndex], runConnectionId)}
        />
      </div>
    </div>
  )
}

// ─── Results sidebar ─────────────────────────────────────────────────────────

const STATUS_ICON: Record<QueryResult['status'], { name: AppIcon; className: string }> = {
  idle: { name: 'loading-03', className: 'text-muted-foreground/50' },
  pending: { name: 'loading-03', className: 'text-muted-foreground/50' },
  running: { name: 'loading-03', className: 'animate-spin text-primary' },
  ok: { name: 'checkmark-circle-02', className: 'text-green-500' },
  error: { name: 'cancel-01', className: 'text-destructive' },
  cancelled: { name: 'cancel-01', className: 'text-muted-foreground' },
  skipped: { name: 'cancel-01', className: 'text-muted-foreground/60' },
}

function resultSummary(result: QueryResult): string {
  switch (result.status) {
    case 'idle':
      return ''
    case 'pending':
      return 'Queued'
    case 'running':
      return 'Running…'
    case 'cancelled':
      return 'Cancelled'
    case 'skipped':
      return 'Skipped'
    case 'error':
      return 'Failed'
    case 'ok': {
      const rowsAffected = result.data.rows_affected
      const count =
        rowsAffected !== undefined
          ? `${rowsAffected} ${rowsAffected === 1 ? 'row' : 'rows'} affected`
          : `${result.data.rows?.length ?? 0} rows`
      return `${count} · ${result.durationMs}ms`
    }
  }
}

function resultLabel(result: QueryResult, index: number): string {
  return 'sql' in result && result.sql
    ? result.sql.replace(/\s+/g, ' ').trim()
    : `Statement ${index + 1}`
}

// ─── Run tabs ─────────────────────────────────────────────────────────────────

function runStatus(results: QueryResult[]): QueryResult['status'] {
  if (results.some((r) => r.status === 'running')) return 'running'
  if (results.some((r) => r.status === 'pending')) return 'pending'
  if (results.some((r) => r.status === 'error')) return 'error'
  if (results.some((r) => r.status === 'cancelled')) return 'cancelled'
  if (results.length > 0 && results.every((r) => r.status === 'skipped')) return 'skipped'
  return 'ok'
}

function runTabLabel(run: ResultRun, index: number): string {
  const first = run.results[0]
  const sql = first && 'sql' in first ? first.sql : undefined
  return sql ? sql.replace(/\s+/g, ' ').trim() : `Run ${index + 1}`
}

const RESULTS_PANEL_MODE_LABEL: Record<ResultsPanelMode, string> = {
  shared: 'Shared',
  'per-connection': 'Per connection',
  'per-editor': 'Per editor',
}

const RESULTS_PANEL_MODE_OPTIONS: { mode: ResultsPanelMode; label: string; hint: string }[] = [
  { mode: 'shared', label: 'Shared', hint: 'One results list for every tab' },
  { mode: 'per-connection', label: 'Per connection', hint: "Follows the focused tab's connection" },
  { mode: 'per-editor', label: 'Per editor', hint: 'Follows the focused editor tab' },
]

function ResultsPanelModeMenu({
  mode,
  onChange,
}: {
  mode: ResultsPanelMode
  onChange: (mode: ResultsPanelMode) => void
}) {
  return (
    <DropdownMenu>
      <Tip label={`Results scope: ${RESULTS_PANEL_MODE_LABEL[mode]}`}>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Change results panel scope"
            />
          }
        >
          <Icon name="settings-05" size={14} />
        </DropdownMenuTrigger>
      </Tip>
      <DropdownMenuContent align="end" className="w-56">
        {RESULTS_PANEL_MODE_OPTIONS.map((option) => (
          <DropdownMenuItem key={option.mode} onClick={() => onChange(option.mode)}>
            <Icon
              name="tick-02"
              size={13}
              data-icon="inline-start"
              className={cn(option.mode !== mode && 'invisible')}
            />
            <div className="flex min-w-0 flex-col">
              <span>{option.label}</span>
              <span className="truncate text-[10px] text-muted-foreground">{option.hint}</span>
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function RunTabStrip({
  runs,
  connections,
  tabs,
  mode,
  activeRunId,
  onSelect,
  onClose,
  onCloseMany,
  onTogglePin,
}: {
  runs: ResultRun[]
  connections: Connection[]
  tabs: EditorTab[]
  mode: ResultsPanelMode
  activeRunId?: string
  onSelect: (runId: string) => void
  onClose: (runId: string) => void
  onCloseMany: (runIds: string[]) => void
  onTogglePin: (runId: string) => void
}) {
  const openContextMenu = useContextMenuOpener()
  const allIds = runs.map((r) => r.id)
  const pinnedIds = new Set(runs.filter((r) => r.pinned).map((r) => r.id))

  // Bulk-close scopes exclude pinned runs — a pin protects a run from
  // "close others"/"to the right"/"to the left" the same way it protects it
  // from history-cap eviction (see resultRunHistory.ts).
  function unpinnedScope(scope: TabCloseScope, runId: string): string[] {
    return tabsToClose(scope, allIds, runId).filter((id) => !pinnedIds.has(id))
  }

  function openTabMenu(run: ResultRun, e: React.MouseEvent) {
    const others = unpinnedScope('others', run.id)
    const right = unpinnedScope('right', run.id)
    const left = unpinnedScope('left', run.id)
    openContextMenu(
      buildResultTabMenu({
        pinned: Boolean(run.pinned),
        hasOthers: others.length > 0,
        hasRight: right.length > 0,
        hasLeft: left.length > 0,
        onClose: () => onClose(run.id),
        onCloseOthers: () => onCloseMany(others),
        onCloseRight: () => onCloseMany(right),
        onCloseLeft: () => onCloseMany(left),
        onTogglePin: () => onTogglePin(run.id),
      }),
      e,
    )
  }

  return (
    <div
      role="tablist"
      aria-label="Runs"
      className="flex h-8 min-w-0 flex-1 items-center gap-0 overflow-x-auto"
    >
      {runs.map((run, index) => {
        const status = runStatus(run.results)
        const icon = STATUS_ICON[status]
        const selected = run.id === activeRunId
        const connection = connections.find((c) => c.id === run.connectionId)
        const sourceTabTitle =
          mode !== 'per-editor' ? tabs.find((t) => t.id === run.tabId)?.title : undefined
        return (
          <div
            key={run.id}
            role="tab"
            aria-selected={selected}
            title={sourceTabTitle}
            onClick={() => onSelect(run.id)}
            onContextMenu={(e) => openTabMenu(run, e)}
            className={cn(
              'group relative flex h-8 max-w-40 shrink-0 cursor-pointer items-center gap-1.5 border-r border-border px-2.5 text-xs',
              selected
                ? 'bg-card text-foreground after:absolute after:left-0 after:right-0 after:top-0 after:h-[2px] after:bg-primary'
                : 'text-muted-foreground hover:bg-card/50 hover:text-foreground',
            )}
          >
            <Icon name={icon.name} size={11} className={cn('shrink-0', icon.className)} />
            {run.pinned && (
              <Icon name="pin-01" size={10} className="shrink-0 text-muted-foreground" />
            )}
            {connection && (
              <span className="shrink-0" title={connection.name}>
                <DriverBadge driver={connection.driver} size="sm" className="size-3" />
              </span>
            )}
            <span className="min-w-0 flex-1 truncate">{runTabLabel(run, index)}</span>
            <button
              type="button"
              aria-label={`Close run ${index + 1}`}
              onClick={(e) => {
                e.stopPropagation()
                onClose(run.id)
              }}
              className={cn(
                'flex size-4 shrink-0 items-center justify-center rounded transition-colors hover:bg-muted hover:text-foreground',
                selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100',
              )}
            >
              <Icon name="cancel-01" size={10} />
            </button>
          </div>
        )
      })}
    </div>
  )
}

function ResultsSidebar({
  results,
  selectedIndex,
  onSelect,
}: {
  results: QueryResult[]
  selectedIndex: number
  onSelect: (index: number) => void
}) {
  return (
    <div
      role="listbox"
      aria-label="Statement results"
      className="flex w-52 shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar"
    >
      {results.map((result, index) => {
        const icon = STATUS_ICON[result.status]
        const selected = index === selectedIndex
        return (
          <button
            key={index}
            type="button"
            role="option"
            aria-selected={selected}
            onClick={() => onSelect(index)}
            className={cn(
              'flex flex-col gap-0.5 border-b border-border px-2.5 py-2 text-left transition-colors hover:bg-accent/40',
              selected && 'bg-accent',
            )}
          >
            <div className="flex items-center gap-1.5">
              <Icon name={icon.name} size={12} className={cn('shrink-0', icon.className)} />
              <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground">
                #{index + 1}
              </span>
              <span className="min-w-0 flex-1 truncate text-[11px] text-foreground">
                {resultLabel(result, index)}
              </span>
            </div>
            <span className="truncate pl-[19px] text-[10px] text-muted-foreground">
              {resultSummary(result)}
            </span>
          </button>
        )
      })}
    </div>
  )
}

function ResultEntry({
  orgSlug,
  workspace,
  activeTabId,
  runId,
  result,
  index,
  connection,
}: {
  orgSlug: string
  workspace: Workspace
  activeTabId?: string
  runId: string
  result: QueryResult
  index: number
  connection: Connection | undefined
}) {
  switch (result.status) {
    case 'idle':
      return <EmptyState />
    case 'pending':
      return <PendingState />
    case 'running':
      return <RunningState />
    case 'cancelled':
      return (
        <CancelledState
          sql={result.sql}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          connection={connection}
        />
      )
    case 'skipped':
      return (
        <SkippedState
          sql={result.sql}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          connection={undefined}
        />
      )
    case 'error':
      return (
        <ErrorState
          sql={result.sql}
          message={result.message}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          connection={connection}
        />
      )
    case 'ok':
      return (
        <ResultSetView
          orgSlug={orgSlug}
          workspace={workspace}
          activeTabId={activeTabId}
          runId={runId}
          result={result}
          index={index}
          connection={connection}
        />
      )
  }
}

function CancelledState({
  sql,
  orgSlug,
  workspaceId,
  connection,
}: {
  sql: string
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
}) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <ResultSqlCaption
        sql={sql}
        orgSlug={orgSlug}
        workspaceId={workspaceId}
        connection={connection}
      />
      <div className="min-h-0 flex-1 overflow-auto p-3">
        <div className="flex items-start gap-2.5 rounded-lg border border-border bg-muted/40 p-3">
          <Icon name="cancel-01" size={14} className="mt-0.5 shrink-0 text-muted-foreground" />
          <div className="flex flex-col gap-0.5">
            <span className="text-xs font-medium text-foreground">Query cancelled</span>
            <span className="text-xs text-muted-foreground">
              The request was stopped before it finished.
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}

function EmptyState() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-center justify-center p-8 text-center">
        <div className="flex flex-col items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-lg border border-border bg-muted/50">
            <Icon name="table" size={17} className="text-muted-foreground" />
          </div>
          <div className="flex flex-col gap-1.5">
            <div className="text-sm font-medium text-foreground">Run a query to see results</div>
            <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground">
              Select a connection and press Run or
              <kbd className="rounded border border-border bg-muted px-1 font-sans text-[10px] leading-4 text-foreground">
                {RUN_SHORTCUT}
              </kbd>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function RunningState() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-center justify-center gap-2 text-xs text-muted-foreground">
        <Icon name="loading-03" size={14} className="animate-spin text-primary" />
        Running query…
      </div>
    </div>
  )
}

function ErrorState({
  sql,
  message,
  orgSlug,
  workspaceId,
  connection,
}: {
  sql: string
  message: string
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
}) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <ResultSqlCaption
        sql={sql}
        orgSlug={orgSlug}
        workspaceId={workspaceId}
        connection={connection}
      />
      <div className="min-h-0 flex-1 overflow-auto p-3">
        <div className="flex items-start gap-2.5 rounded-lg border border-destructive/30 bg-destructive/5 p-3">
          <Icon name="cancel-01" size={14} className="mt-0.5 shrink-0 text-destructive" />
          <div className="flex min-w-0 flex-col gap-1">
            <span className="text-xs font-medium text-destructive">Query failed</span>
            <pre className="whitespace-pre-wrap break-all text-xs text-destructive/90">
              {message}
            </pre>
          </div>
        </div>
      </div>
    </div>
  )
}

const ROW_NUM_COL_WIDTH = 48
const DEFAULT_COL_WIDTH = 150
const MIN_COL_WIDTH = 60
const ROW_HEIGHT = 29

function copyToClipboard(text: string) {
  try {
    if (navigator.clipboard) {
      void navigator.clipboard.writeText(text)
    } else {
      const el = document.createElement('textarea')
      el.value = text
      el.style.cssText = 'position:fixed;opacity:0'
      document.body.appendChild(el)
      el.select()
      document.execCommand('copy')
      document.body.removeChild(el)
    }
  } catch {
    /* ignore */
  }
}

function PendingState() {
  return (
    <div className="flex h-full min-h-0 flex-col items-center justify-center gap-1 bg-card p-6 text-center text-sm text-muted-foreground">
      Queued
    </div>
  )
}

function SkippedState({
  sql,
  orgSlug,
  workspaceId,
  connection,
}: {
  sql: string
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
}) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <ResultSqlCaption
        sql={sql}
        orgSlug={orgSlug}
        workspaceId={workspaceId}
        connection={connection}
      />
      <div className="flex flex-1 items-center justify-center p-6 text-center text-sm text-muted-foreground">
        Skipped — an earlier statement stopped the run.
      </div>
    </div>
  )
}

function ResultSetView({
  orgSlug,
  workspace,
  activeTabId,
  runId,
  result,
  index,
  connection,
}: {
  orgSlug: string
  workspace: Workspace
  activeTabId?: string
  runId: string
  result: Extract<QueryResult, { status: 'ok' }>
  index: number
  connection: Connection | undefined
}) {
  const { durationMs } = result
  const columns = result.data.columns ?? []
  const rows = result.data.rows ?? []
  const hasColumns = columns.length > 0
  const rowsAffected = result.data.rows_affected
  const columnNames = columns.map((c) => c.name)
  const cellText = (v: ResultValue) => formatValue(v).display

  const { columnWidths: colWidths, startResize } = useColumnResize(
    columns.length,
    DEFAULT_COL_WIDTH,
    MIN_COL_WIDTH,
  )

  const [selection, setSelection] = useState<CellSelection | null>(null)
  const [rowSelectionMode, setRowSelectionMode] = useState(false)
  const [tableCollapsed, setTableCollapsed] = useState(false)
  const tablePanelRef = useRef<PanelImperativeHandle>(null)
  const tableContainerRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const isDraggingRef = useRef(false)
  const scrollElRef = useRef<HTMLElement | null>(null)
  const pointerRef = useRef<{ x: number; y: number } | null>(null)
  const autoScrollRafRef = useRef<number | null>(null)
  const tabs = useIde((s) => s.tabs)
  const activeTab = activeTabId ? tabs.find((t) => t.id === activeTabId) : undefined
  const queryCursorId = result.data.query_cursor_id
  const cursorConnectionId = result.connectionId ?? activeTab?.connectionId
  const { handleGridScroll } = useResultCursorPaging({
    activeTabId,
    connectionId: cursorConnectionId,
    index,
    orgSlug,
    result,
    runId,
    workspaceId: workspace.id,
  })

  // Track the pointer while drag-selecting, and stop drag + auto-scroll on mouseup.
  useEffect(() => {
    function onMouseMove(ev: MouseEvent) {
      if (isDraggingRef.current) pointerRef.current = { x: ev.clientX, y: ev.clientY }
    }
    function onMouseUp() {
      isDraggingRef.current = false
      if (autoScrollRafRef.current != null) {
        cancelAnimationFrame(autoScrollRafRef.current)
        autoScrollRafRef.current = null
      }
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
    return () => {
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
      if (autoScrollRafRef.current != null) {
        cancelAnimationFrame(autoScrollRafRef.current)
        autoScrollRafRef.current = null
      }
    }
  }, [])

  // Focus anchor cell after keyboard navigation (skip during mouse drag).
  useEffect(() => {
    if (!selection || isDraggingRef.current) return
    const { rowIdx, colIdx } = selection.anchor
    rowVirtualizer.scrollToIndex(rowIdx)
    requestAnimationFrame(() => {
      tableContainerRef.current
        ?.querySelector<HTMLElement>(`[data-cell="${rowIdx}-${colIdx}"]`)
        ?.focus({ preventScroll: true })
    })
    // rowVirtualizer is a stable instance; intentionally not a dep.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selection])

  function handleTableKeyDown(e: React.KeyboardEvent) {
    if (!selection) return

    if ((e.metaKey || e.ctrlKey) && e.key === 'c') {
      e.preventDefault()
      const minR = Math.min(selection.anchor.rowIdx, selection.active.rowIdx)
      const maxR = Math.max(selection.anchor.rowIdx, selection.active.rowIdx)
      const minC = Math.min(selection.anchor.colIdx, selection.active.colIdx)
      const maxC = Math.max(selection.anchor.colIdx, selection.active.colIdx)
      const text = rows
        .slice(minR, maxR + 1)
        .map((row) =>
          row
            .slice(minC, maxC + 1)
            .map((v) => formatValue(v).display)
            .join('\t'),
        )
        .join('\n')
      copyToClipboard(text)
      return
    }

    const target = nextCell(e.key, selection.anchor, rows.length, columns.length)
    if (!target) return
    e.preventDefault()
    if (target.rowIdx !== selection.anchor.rowIdx || target.colIdx !== selection.anchor.colIdx) {
      setSelection({ anchor: target, active: target })
    }
  }

  // While drag-selecting, scroll the grid when the pointer nears an edge, and
  // extend the selection to whatever cell ends up under the cursor.
  function autoScrollStep() {
    const el = scrollElRef.current
    const p = pointerRef.current
    if (!isDraggingRef.current || !el || !p) {
      autoScrollRafRef.current = null
      return
    }
    const rect = el.getBoundingClientRect()
    const EDGE = 56
    const ramp = (d: number) => Math.min(14, Math.max(0, d) * 0.25)
    let dx = 0
    let dy = 0
    if (p.x < rect.left + EDGE) dx = -ramp(rect.left + EDGE - p.x)
    else if (p.x > rect.right - EDGE) dx = ramp(p.x - (rect.right - EDGE))
    if (p.y < rect.top + EDGE) dy = -ramp(rect.top + EDGE - p.y)
    else if (p.y > rect.bottom - EDGE) dy = ramp(p.y - (rect.bottom - EDGE))
    if (dx || dy) {
      el.scrollBy(dx, dy)
      const cell = (
        document.elementFromPoint(p.x, p.y) as HTMLElement | null
      )?.closest<HTMLElement>('[data-cell]')
      const data = cell?.dataset.cell
      if (data) {
        const [r, c] = data.split('-').map(Number)
        setSelection((prev) =>
          prev && (prev.active.rowIdx !== r || prev.active.colIdx !== c)
            ? { ...prev, active: { rowIdx: r, colIdx: c } }
            : prev,
        )
      }
    }
    autoScrollRafRef.current = requestAnimationFrame(autoScrollStep)
  }

  function startAutoScroll(e: React.MouseEvent) {
    pointerRef.current = { x: e.clientX, y: e.clientY }
    scrollElRef.current = scrollRef.current
    if (autoScrollRafRef.current == null)
      autoScrollRafRef.current = requestAnimationFrame(autoScrollStep)
  }

  // One context menu for the whole grid: cells/rows/headers build their items
  // on right-click and hand them to the shared provider menu. Avoids mounting a
  // menu controller per cell, which makes large result sets slow to render.
  const openContextMenu = useContextMenuOpener()

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  })
  const virtualRows = rowVirtualizer.getVirtualItems()

  function openCellMenu(rowIdx: number, colIdx: number, e: React.MouseEvent) {
    const v = rows[rowIdx]?.[colIdx]
    const { display, isNull } = v ? formatValue(v) : { display: '', isNull: true }
    openContextMenu(
      buildCellMenu({
        onCopyValue: () => copyWithToast(isNull ? 'NULL' : display),
        onCopyColumnName: () => copyWithToast(columns[colIdx]?.name ?? ''),
      }),
      e,
    )
  }

  function openRowMenu(rowIdx: number, e: React.MouseEvent) {
    const row = rows[rowIdx] ?? []
    openContextMenu(
      buildRowMenu({
        onCopyRow: () => copyWithToast(rowToTsv(row.map(cellText))),
        onCopyRowJson: () => copyWithToast(rowToJson(columnNames, row.map(cellText))),
      }),
      e,
    )
  }

  function openColumnMenu(colIdx: number, e: React.MouseEvent) {
    openContextMenu(
      buildColumnHeaderMenu({
        onCopyName: () => copyWithToast(columns[colIdx]?.name ?? ''),
        onCopyAllValues: () => copyWithToast(valuesToLines(rows.map((r) => cellText(r[colIdx])))),
      }),
      e,
    )
  }

  function handleCellMouseDown(ri: number, ci: number, e: React.MouseEvent) {
    if (e.button !== 0) return // ignore right/middle click (context menu handles right-click)
    e.preventDefault() // suppress browser text-selection drag (also suppresses auto-focus)
    isDraggingRef.current = true
    startAutoScroll(e)
    setRowSelectionMode(false)
    setSelection({ anchor: { rowIdx: ri, colIdx: ci }, active: { rowIdx: ri, colIdx: ci } })
    setTableCollapsed(false)
    tablePanelRef.current?.expand()
    // Restore focus manually since preventDefault() suppressed it — required for keyboard events.
    tableContainerRef.current
      ?.querySelector<HTMLElement>(`[data-cell="${ri}-${ci}"]`)
      ?.focus({ preventScroll: false })
  }

  function handleCellDragEnter(ri: number, ci: number) {
    if (!isDraggingRef.current) return
    setSelection((prev) => (prev ? { ...prev, active: { rowIdx: ri, colIdx: ci } } : null))
  }

  function closePanel() {
    setSelection(null)
    setRowSelectionMode(false)
    setTableCollapsed(false)
  }

  function handleRowHeaderMouseDown(ri: number, e: React.MouseEvent) {
    if (e.button !== 0) return // ignore right/middle click (context menu handles right-click)
    e.preventDefault()
    setRowSelectionMode(true)
    setSelection({
      anchor: { rowIdx: ri, colIdx: 0 },
      active: { rowIdx: ri, colIdx: columns.length - 1 },
    })
  }

  const totalWidth = ROW_NUM_COL_WIDTH + colWidths.reduce((a, b) => a + b, 0)

  const panelValue = selection
    ? rows[selection.anchor.rowIdx]?.[selection.anchor.colIdx]
    : undefined
  const panelCol = selection ? columns[selection.anchor.colIdx] : undefined

  if (!hasColumns) {
    return (
      <div className="flex h-full min-h-0 flex-col bg-card">
        <ResultSqlCaption
          sql={result.sql}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          connection={connection}
        />
        <div className="flex min-h-0 flex-1 items-center justify-center">
          <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            <Icon name="checkmark-circle-02" size={14} className="text-green-500" />
            <span className="font-medium text-foreground">Query executed</span>
            {rowsAffected !== undefined && (
              <span className="tabular-nums">
                · {rowsAffected} {rowsAffected === 1 ? 'row' : 'rows'} affected
              </span>
            )}
            <span className="tabular-nums">· {durationMs}ms</span>
          </div>
        </div>
      </div>
    )
  }

  const tableEl = (
    <div ref={tableContainerRef} onKeyDown={handleTableKeyDown} className="select-none">
      <table
        role="grid"
        className="table-fixed border-separate border-spacing-0 text-xs"
        style={{ width: totalWidth }}
      >
        <thead className="sticky top-0 z-10 bg-muted shadow-[0_-1px_0_0_var(--color-muted)]">
          <tr role="row">
            <th
              role="columnheader"
              style={{ width: ROW_NUM_COL_WIDTH }}
              className="sticky left-0 z-20 border-b border-r border-border bg-muted px-2 py-1.5 text-right font-medium text-muted-foreground tabular-nums"
            />
            {columns.map((col, i) => (
              <ColumnHeader
                key={i}
                col={col}
                width={colWidths[i]}
                onResizeStart={(e) => startResize(e, i)}
                onContextMenu={(e) => openColumnMenu(i, e)}
              />
            ))}
          </tr>
        </thead>
        <tbody>
          {virtualRows.length > 0 && (
            <tr aria-hidden style={{ height: virtualRows[0].start }}>
              <td colSpan={columns.length + 1} className="p-0" />
            </tr>
          )}
          {virtualRows.map((vr) => (
            <DataRow
              key={vr.index}
              row={rows[vr.index]}
              columns={columns}
              rowIdx={vr.index}
              selection={selection}
              rowSelectionMode={rowSelectionMode}
              onRowHeaderMouseDown={(e) => handleRowHeaderMouseDown(vr.index, e)}
              onRowHeaderContextMenu={(e) => openRowMenu(vr.index, e)}
              onCellMouseDown={(ci, e) => handleCellMouseDown(vr.index, ci, e)}
              onCellMouseEnter={(ci) => handleCellDragEnter(vr.index, ci)}
              onCellContextMenu={(ci, e) => openCellMenu(vr.index, ci, e)}
            />
          ))}
          {virtualRows.length > 0 && (
            <tr
              aria-hidden
              style={{
                height: rowVirtualizer.getTotalSize() - virtualRows[virtualRows.length - 1].end,
              }}
            >
              <td colSpan={columns.length + 1} className="p-0" />
            </tr>
          )}
          {rows.length === 0 && (
            <tr role="row">
              <td
                colSpan={columns.length + 1}
                className="border-b border-border px-4 py-3 text-center text-muted-foreground"
              >
                No rows returned
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )

  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <ResultSqlCaption
        sql={result.sql}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        connection={connection}
        showConnection={false}
      />
      {/*
       * Always render the table inside ResizablePanelGroup so the table's
       * scroll container is a stable DOM element. Switching between a plain
       * <div> and a ResizablePanel on first cell click was resetting the
       * scroll position to the top because the new container starts at 0.
       */}
      <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
        <ResizablePanel
          panelRef={tablePanelRef}
          defaultSize="75%"
          minSize="15%"
          collapsible
          collapsedSize="0%"
          className="min-h-0 overflow-hidden"
          onResize={(size) => setTableCollapsed(size.asPercentage === 0)}
        >
          <div ref={scrollRef} className="h-full overflow-auto" onScroll={handleGridScroll}>
            {tableEl}
          </div>
        </ResizablePanel>
        {selection && panelValue && panelCol && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel
              defaultSize="25%"
              minSize="15%"
              className="flex flex-col border-l border-border"
            >
              <CellDetailPanel
                value={panelValue}
                col={panelCol}
                tableCollapsed={tableCollapsed}
                onMaximize={() =>
                  tableCollapsed
                    ? tablePanelRef.current?.expand()
                    : tablePanelRef.current?.collapse()
                }
                onClose={closePanel}
              />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>

      <div className="flex h-6 shrink-0 items-center border-t border-border bg-sidebar px-3 text-[11px] text-muted-foreground">
        {connection && (
          <>
            <span
              className="flex min-w-0 max-w-32 shrink-0 items-center gap-1 tabular-nums"
              title={connection.name}
            >
              <DriverBadge driver={connection.driver} size="sm" className="size-3 shrink-0" />
              <span className="min-w-0 truncate">{connection.name}</span>
            </span>
            <span className="mx-1.5 shrink-0 opacity-40">·</span>
          </>
        )}
        <span className="shrink-0 tabular-nums">
          {rows.length === 1 ? '1 row' : `${rows.length} rows`}
          {queryCursorId ? ' fetched' : ''}
        </span>
        <span className="mx-1.5 shrink-0 opacity-40">·</span>
        <span className="shrink-0 tabular-nums">{durationMs}ms</span>
        {result.isFetchingNextPage && (
          <>
            <span className="mx-1.5 shrink-0 opacity-40">·</span>
            <span className="shrink-0">Loading more…</span>
          </>
        )}
        {result.cursorMessage && (
          <>
            <span className="mx-1.5 shrink-0 opacity-40">·</span>
            <span className="min-w-0 truncate">{result.cursorMessage}</span>
          </>
        )}
      </div>
    </div>
  )
}

// ─── SQL caption ──────────────────────────────────────────────────────────────

/** Slim strip above each result set naming the query it came from. */
function ResultSqlCaption({
  sql,
  orgSlug,
  workspaceId,
  connection,
  showConnection = true,
}: {
  sql: string
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
  showConnection?: boolean
}) {
  const [viewQueryOpen, setViewQueryOpen] = useState(false)

  if (!sql) return null
  return (
    <>
      <div
        role="button"
        tabIndex={0}
        onClick={() => setViewQueryOpen(true)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setViewQueryOpen(true)
          }
        }}
        className="flex h-7 shrink-0 cursor-pointer items-center gap-2 border-b border-border bg-muted/30 pl-3 pr-1.5 hover:bg-muted/50"
      >
        <Icon name="terminal" size={11} className="shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground" title={sql}>
          {sql.replace(/\s+/g, ' ').trim()}
        </span>
        {showConnection && connection && (
          <span
            className="flex min-w-0 max-w-32 shrink-0 items-center gap-1 text-[11px] text-muted-foreground"
            title={connection.name}
          >
            <DriverBadge driver={connection.driver} size="sm" className="size-3 shrink-0" />
            <span className="min-w-0 truncate">{connection.name}</span>
          </span>
        )}
        <span onClick={(e) => e.stopPropagation()} className="flex shrink-0 items-center">
          <ExportButton
            orgSlug={orgSlug}
            workspaceId={workspaceId}
            connectionId={connection?.id}
            getSql={() => sql}
            className="scale-90"
          />
          <Tip label="Copy query">
            <button
              type="button"
              aria-label="Copy query"
              onClick={() => copyWithToast(sql)}
              className="flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <Icon name="copy-01" size={11} />
            </button>
          </Tip>
        </span>
      </div>
      <ViewQueryDialog open={viewQueryOpen} onOpenChange={setViewQueryOpen} sql={sql} />
    </>
  )
}

// ─── Table sub-components ────────────────────────────────────────────────────

function RowHeaderCell({
  label,
  selected,
  onMouseDown,
  onContextMenu,
}: {
  label: number
  selected: boolean
  onMouseDown: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  return (
    <td
      role="rowheader"
      onMouseDown={onMouseDown}
      onContextMenu={onContextMenu}
      className={cn(
        'sticky left-0 z-[5] cursor-pointer border-b border-r border-border px-2 py-1 text-right text-muted-foreground tabular-nums',
        selected ? 'bg-primary/15' : 'bg-card',
      )}
    >
      {label}
    </td>
  )
}

function DataRow({
  row,
  columns,
  rowIdx,
  selection,
  rowSelectionMode,
  onRowHeaderMouseDown,
  onRowHeaderContextMenu,
  onCellMouseDown,
  onCellMouseEnter,
  onCellContextMenu,
}: {
  row: ResultValue[]
  columns: ResultColumn[]
  rowIdx: number
  selection: CellSelection | null
  rowSelectionMode: boolean
  onRowHeaderMouseDown: (e: React.MouseEvent) => void
  onRowHeaderContextMenu: (e: React.MouseEvent) => void
  onCellMouseDown: (ci: number, e: React.MouseEvent) => void
  onCellMouseEnter: (ci: number) => void
  onCellContextMenu: (ci: number, e: React.MouseEvent) => void
}) {
  return (
    <tr role="row" className="group" style={{ height: ROW_HEIGHT }}>
      <RowHeaderCell
        label={rowIdx + 1}
        selected={rowSelectionMode && isRowInRange(rowIdx, selection)}
        onMouseDown={onRowHeaderMouseDown}
        onContextMenu={onRowHeaderContextMenu}
      />
      {row.map((val, ci) => (
        <DataCell
          key={ci}
          value={val}
          col={columns[ci]}
          rowIdx={rowIdx}
          colIdx={ci}
          isAnchor={selection?.anchor.rowIdx === rowIdx && selection?.anchor.colIdx === ci}
          isInRange={cellInRange(rowIdx, ci, selection)}
          onMouseDown={(e) => onCellMouseDown(ci, e)}
          onMouseEnter={() => onCellMouseEnter(ci)}
          onContextMenu={(e) => onCellContextMenu(ci, e)}
        />
      ))}
    </tr>
  )
}

function ColumnHeader({
  col,
  width,
  onResizeStart,
  onContextMenu,
}: {
  col: ResultColumn
  width: number
  onResizeStart: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  const icon = columnTypeIcon(col.type)
  return (
    <th
      style={{ width }}
      onContextMenu={onContextMenu}
      title={`${col.name} · ${col.type}`}
      className="relative border-b border-r border-border px-2.5 py-1.5 text-left font-medium select-none overflow-hidden"
    >
      <div className="flex items-center gap-1.5">
        <Icon
          name={icon}
          size={13}
          className={cn('shrink-0', columnTypeIconColor[icon] ?? 'text-muted-foreground')}
        />
        <div className="flex min-w-0 flex-col">
          <span className="truncate leading-tight text-foreground">{col.name}</span>
          <span className="truncate text-[9px] font-normal uppercase leading-tight tracking-wider text-muted-foreground/70">
            {col.type}
          </span>
        </div>
      </div>
      <div
        className="absolute inset-y-0 right-0 w-[2px] cursor-col-resize hover:bg-primary/50"
        onMouseDown={onResizeStart}
      />
    </th>
  )
}

function DataCell({
  value,
  col,
  rowIdx,
  colIdx,
  isAnchor,
  isInRange,
  onMouseDown,
  onMouseEnter,
  onContextMenu,
}: {
  value: ResultValue
  col: ResultColumn
  rowIdx: number
  colIdx: number
  isAnchor: boolean
  isInRange: boolean
  onMouseDown: (e: React.MouseEvent) => void
  onMouseEnter: () => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  const { display, isNull, isNumeric } = formatValue(value)
  const isRightAlign = isNumeric || col.type === 'integer' || col.type === 'decimal'

  return (
    <td
      role="gridcell"
      data-cell={`${rowIdx}-${colIdx}`}
      tabIndex={isAnchor ? 0 : -1}
      onMouseDown={onMouseDown}
      onMouseEnter={onMouseEnter}
      onContextMenu={onContextMenu}
      className={cn(
        'max-w-0 cursor-default overflow-hidden border-b border-r border-border px-3 py-1 outline-none',
        isRightAlign ? 'text-right tabular-nums' : 'text-left',
        isNull ? 'text-muted-foreground/50' : '',
        isInRange ? 'bg-primary/15' : 'group-hover:bg-accent/30',
        isAnchor && 'ring-1 ring-inset ring-primary/60',
      )}
    >
      {isNull ? (
        <span className="italic">NULL</span>
      ) : (
        <span className="block overflow-hidden text-ellipsis whitespace-nowrap">{display}</span>
      )}
    </td>
  )
}

// ─── Cell detail panel ────────────────────────────────────────────────────────

function CellDetailPanel({
  value,
  col,
  tableCollapsed,
  onMaximize,
  onClose,
}: {
  value: ResultValue
  col: ResultColumn
  tableCollapsed: boolean
  onMaximize: () => void
  onClose: () => void
}) {
  const [copied, setCopied] = useState(false)
  const { display, isNull } = formatValue(value)

  function handleCopy() {
    copyToClipboard(isNull ? 'NULL' : display)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  // Parent ResizablePanel is flex flex-col — header + content fill it directly.
  return (
    <>
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2">
        <div className="flex min-w-0 flex-1 items-center gap-1.5">
          <Icon
            name={columnTypeIcon(col.type)}
            size={12}
            className={cn(
              'shrink-0',
              columnTypeIconColor[columnTypeIcon(col.type)] ?? 'text-muted-foreground',
            )}
          />
          <span className="truncate text-xs font-medium text-foreground">{col.name}</span>
          <span className="shrink-0 text-[9px] font-normal uppercase tracking-wider text-muted-foreground/70">
            {col.type}
          </span>
        </div>
        <div className="relative">
          {copied && (
            <div className="pointer-events-none absolute right-full top-1/2 mr-1.5 -translate-y-1/2 whitespace-nowrap rounded bg-foreground px-1.5 py-0.5 text-[10px] leading-tight text-background">
              Copied
            </div>
          )}
          <Tip label="Copy value">
            <button
              type="button"
              onClick={handleCopy}
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
              aria-label="Copy value"
            >
              <Icon name={copied ? 'tick-02' : 'copy-01'} size={13} />
            </button>
          </Tip>
        </div>
        <Tip label={tableCollapsed ? 'Restore grid' : 'Maximize value panel'}>
          <button
            type="button"
            onClick={onMaximize}
            className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label={tableCollapsed ? 'Restore' : 'Maximize'}
          >
            <Icon name={tableCollapsed ? 'minimize' : 'maximize'} size={13} />
          </button>
        </Tip>
        <Tip label="Close value panel">
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label="Close"
          >
            <Icon name="cancel-01" size={13} />
          </button>
        </Tip>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-3">
        <CellContent display={display} isNull={isNull} col={col} />
      </div>
    </>
  )
}

function CellContent({
  display,
  isNull,
  col: _col,
}: {
  display: string
  isNull: boolean
  col: ResultColumn
}) {
  if (isNull) {
    return <span className="text-xs italic text-muted-foreground">NULL</span>
  }
  // Future: switch on _col.type to add datetime parsed view, json pretty-print, etc.
  return (
    <pre className="whitespace-pre-wrap break-words text-xs leading-relaxed text-foreground">
      {display}
    </pre>
  )
}

// ─── Value formatter ─────────────────────────────────────────────────────────
