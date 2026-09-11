import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { cn } from '#/lib/utils'
import type { Connection, Workspace } from '#/lib/api/types'
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
import { copyWithToast } from './contextMenus/clipboard'
import { buildCellMenu, buildRowMenu, buildColumnHeaderMenu } from './contextMenus/resultMenu'
import { buildResultTabMenu } from './contextMenus/resultTabMenu'
import { tabsToClose, type TabCloseScope } from './ideLayout'
import { RUN_SHORTCUT } from './IdeToolbar'
import { Tip } from './schema-diagram/Tip'
import { DriverBadge } from './DriverBadge'
import { ExportButton } from './exports/ExportButton'
import { ViewQueryDialog } from './ViewQueryDialog'
import { allOrgWorkspaceConnectionsQueryOptions } from '#/lib/api/query'
import { useResultCursorPaging } from './useResultCursorPaging'
import { DataGrid } from './dataGrid/DataGrid'

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
          key={`${runId}:0`}
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
        {/* Keyed per statement so switching the selected statement remounts
            the grid instead of reusing the scroll container — otherwise the
            new statement's rows render at whatever scrollTop the previous
            statement left behind rather than always starting at the top. */}
        <ResultEntry
          key={`${runId}:${selectedIndex}`}
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
  ok: { name: 'checkmark-circle-02', className: 'text-success' },
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
    const all = unpinnedScope('all', run.id)
    openContextMenu(
      buildResultTabMenu({
        pinned: Boolean(run.pinned),
        hasOthers: others.length > 0,
        hasRight: right.length > 0,
        hasLeft: left.length > 0,
        hasAll: all.length > 0,
        onClose: () => onClose(run.id),
        onCloseOthers: () => onCloseMany(others),
        onCloseRight: () => onCloseMany(right),
        onCloseLeft: () => onCloseMany(left),
        onCloseAll: () => onCloseMany(all),
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
        <div className="flex items-start gap-2.5 rounded-lg bg-muted/40 p-3">
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
          <div className="flex size-10 items-center justify-center rounded-lg bg-muted/50">
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
  const queryCursorId = result.data.query_cursor_id

  const tabs = useIde((s) => s.tabs)
  const activeTab = activeTabId ? tabs.find((t) => t.id === activeTabId) : undefined
  const cursorConnectionId = result.connectionId ?? activeTab?.connectionId
  const { canFetchMore, fetchNextPage } = useResultCursorPaging({
    activeTabId,
    connectionId: cursorConnectionId,
    index,
    orgSlug,
    result,
    runId,
    workspaceId: workspace.id,
  })

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
          <div className="flex items-center gap-2 rounded-lg bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            <Icon name="checkmark-circle-02" size={14} className="text-success" />
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

  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <ResultSqlCaption
        sql={result.sql}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        connection={connection}
        showConnection={false}
      />
      <DataGrid
        columns={columns}
        rows={rows}
        onScrollNearEnd={canFetchMore ? fetchNextPage : undefined}
        isLoadingMore={result.isFetchingNextPage}
        buildCellMenu={buildCellMenu}
        buildRowMenu={buildRowMenu}
        buildColumnHeaderMenu={buildColumnHeaderMenu}
      />
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
