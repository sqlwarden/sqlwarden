import { useState, useRef, useEffect, type UIEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '#/components/ui/resizable'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { cn } from '#/lib/utils'
import type { ResultColumn, ResultSet, ResultValue, Workspace } from '#/lib/api/types'
import { useIde, activeTabId as selectActiveTabId, type QueryResult } from './useIdeStore'
import { useContextMenuOpener } from '#/components/ui/context-menu'
import { api } from '#/lib/api/client'
import { ApiError } from '#/lib/api/errors'
import { copyWithToast, rowToTsv, rowToJson, valuesToLines } from './contextMenus/clipboard'
import { buildCellMenu, buildRowMenu, buildColumnHeaderMenu } from './contextMenus/resultMenu'
import { nextCell } from './resultGridNav'

type ResultsAreaProps = {
  orgSlug: string
  workspace: Workspace
}

export function ResultsArea({ orgSlug, workspace }: ResultsAreaProps) {
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const activeTabId = useIde((s) => (s.activeWorkspaceId ? selectActiveTabId(s, s.activeWorkspaceId) : undefined))
  const results = useIde((s) => s.results)

  const result: QueryResult = activeTabId ? (results[activeTabId] ?? { status: 'idle' }) : { status: 'idle' }

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'results' ? null : 'results')
  }

  return (
    <Tabs defaultValue="results" className="flex min-h-0 flex-1 flex-col gap-0">
      <div className="flex h-9 shrink-0 items-center justify-between border-b border-border bg-background px-1">
        <TabsList variant="line" className="h-full gap-0 rounded-none p-0">
          <TabsTrigger
            value="results"
            className="h-full rounded-none px-3 text-xs gap-1.5"
          >
            <Icon name="table" size={13} />
            Results
            {result.status === 'ok' && (result.data.rows?.length ?? 0) > 0 && (
              <span className="ml-0.5 rounded bg-primary/10 px-1 text-[10px] font-medium text-primary tabular-nums">
                {result.data.rows?.length}
              </span>
            )}
          </TabsTrigger>
          <TabsTrigger value="history" className="h-full rounded-none px-3 text-xs">
            History
          </TabsTrigger>
          <TabsTrigger value="explain" className="h-full rounded-none px-3 text-xs">
            Explain
          </TabsTrigger>
        </TabsList>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Toggle results maximize"
          onClick={toggleMaximize}
        >
          <Icon
            name={maximizedPane === 'results' ? 'minimize' : 'maximize'}
            size={14}
          />
        </Button>
      </div>

      <TabsContent value="results" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <ResultsContent key={activeTabId} orgSlug={orgSlug} workspace={workspace} activeTabId={activeTabId} result={result} />
      </TabsContent>
      <TabsContent value="history" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <StubPane message="Query history coming soon." />
      </TabsContent>
      <TabsContent value="explain" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <StubPane message="Execution plan coming soon." />
      </TabsContent>
    </Tabs>
  )
}

// ─── Result content switcher ────────────────────────────────────────────────

function ResultsContent({
  orgSlug,
  workspace,
  activeTabId,
  result,
}: {
  orgSlug: string
  workspace: Workspace
  activeTabId?: string
  result: QueryResult
}) {
  switch (result.status) {
    case 'idle':
      return <EmptyState />
    case 'running':
      return <RunningState />
    case 'cancelled':
      return <CancelledState />
    case 'error':
      return <ErrorState message={result.message} />
    case 'ok':
      return <OkState orgSlug={orgSlug} workspace={workspace} activeTabId={activeTabId} result={result} />
  }
}

function CancelledState() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-start gap-2.5 overflow-auto p-4">
        <Icon name="cancel-01" size={14} className="mt-0.5 shrink-0 text-muted-foreground" />
        <span className="text-xs text-muted-foreground">Query cancelled.</span>
      </div>
    </div>
  )
}

function EmptyState() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-center justify-center p-8 text-center">
        <div className="flex flex-col gap-1.5">
          <div className="text-sm font-medium text-foreground">Run a query to see results</div>
          <div className="text-xs text-muted-foreground">Select a connection and press Run or ⌘↵</div>
        </div>
      </div>
    </div>
  )
}

function RunningState() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-center justify-center gap-2 text-xs text-muted-foreground">
        <Icon name="loading-03" size={14} className="animate-spin" />
        Running…
      </div>
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-start gap-2.5 overflow-auto p-4">
        <Icon name="cancel-01" size={14} className="mt-0.5 shrink-0 text-destructive" />
        <pre className="whitespace-pre-wrap break-all font-mono text-xs text-destructive">{message}</pre>
      </div>
    </div>
  )
}

const ROW_NUM_COL_WIDTH = 48
const DEFAULT_COL_WIDTH = 150
const MIN_COL_WIDTH = 60
const ROW_HEIGHT = 29

type CellCoord = { rowIdx: number; colIdx: number }
type CellSelection = { anchor: CellCoord; active: CellCoord }

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
  } catch { /* ignore */ }
}

function cellInRange(ri: number, ci: number, sel: CellSelection | null): boolean {
  if (!sel) return false
  const minR = Math.min(sel.anchor.rowIdx, sel.active.rowIdx)
  const maxR = Math.max(sel.anchor.rowIdx, sel.active.rowIdx)
  const minC = Math.min(sel.anchor.colIdx, sel.active.colIdx)
  const maxC = Math.max(sel.anchor.colIdx, sel.active.colIdx)
  return ri >= minR && ri <= maxR && ci >= minC && ci <= maxC
}

function isRowInRange(ri: number, sel: CellSelection | null): boolean {
  if (!sel) return false
  const minR = Math.min(sel.anchor.rowIdx, sel.active.rowIdx)
  const maxR = Math.max(sel.anchor.rowIdx, sel.active.rowIdx)
  return ri >= minR && ri <= maxR
}

function OkState({
  orgSlug,
  workspace,
  activeTabId,
  result,
}: {
  orgSlug: string
  workspace: Workspace
  activeTabId?: string
  result: Extract<QueryResult, { status: 'ok' }>
}) {
  const { durationMs } = result
  const columns = result.data.columns ?? []
  const rows = result.data.rows ?? []
  const hasColumns = columns.length > 0
  const columnNames = columns.map((c) => c.name)
  const cellText = (v: ResultValue) => formatValue(v).display

  const [colWidths, setColWidths] = useState<number[]>(() => columns.map(() => DEFAULT_COL_WIDTH))
  const resizingRef = useRef<{ colIdx: number; startX: number; startWidth: number } | null>(null)

  const [selection, setSelection] = useState<CellSelection | null>(null)
  const [rowSelectionMode, setRowSelectionMode] = useState(false)
  const [tableCollapsed, setTableCollapsed] = useState(false)
  const tablePanelRef = useRef<PanelImperativeHandle>(null)
  const tableContainerRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const fetchingNextPageRef = useRef(false)
  const isDraggingRef = useRef(false)
  const scrollElRef = useRef<HTMLElement | null>(null)
  const pointerRef = useRef<{ x: number; y: number } | null>(null)
  const autoScrollRafRef = useRef<number | null>(null)
  const tabs = useIde((s) => s.tabs)
  const setQueryResult = useIde((s) => s.setQueryResult)
  const activeTab = activeTabId ? tabs.find((t) => t.id === activeTabId) : undefined
  const queryCursorId = result.data.query_cursor_id
  const canFetchMore = Boolean(queryCursorId && result.data.exhausted === false && !result.isFetchingNextPage && activeTab?.connectionId)

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
        .map(row => row.slice(minC, maxC + 1).map(v => formatValue(v).display).join('\t'))
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

  async function fetchNextPage() {
    if (!activeTabId || !activeTab?.connectionId || !queryCursorId || !canFetchMore || fetchingNextPageRef.current) return
    fetchingNextPageRef.current = true
    setQueryResult(activeTabId, { ...result, isFetchingNextPage: true, cursorMessage: undefined })
    try {
      const page = await api.post<ResultSet>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${activeTab.connectionId}/query-cursors/${queryCursorId}/fetch`,
        { page_size: result.data.page_size },
      )
      const nextRows = [...(result.data.rows ?? []), ...(page.rows ?? [])]
      const exhausted = page.exhausted ?? true
      const nextCursorId = exhausted ? undefined : (page.query_cursor_id ?? queryCursorId)
      setQueryResult(activeTabId, {
        ...result,
        isFetchingNextPage: false,
        data: {
          ...result.data,
          rows: nextRows,
          rows_returned: nextRows.length,
          bytes_returned: result.data.bytes_returned + page.bytes_returned,
          truncated: result.data.truncated || page.truncated,
          truncation_reason: page.truncation_reason ?? result.data.truncation_reason,
          query_cursor_id: nextCursorId,
          exhausted,
          page_size: page.page_size ?? result.data.page_size,
        },
      })
    } catch (err) {
      const expired = err instanceof ApiError && err.code === 'query_cursor_unavailable'
      setQueryResult(activeTabId, {
        ...result,
        isFetchingNextPage: false,
        cursorMessage: expired ? 'Cursor expired. Run the query again.' : (err instanceof Error ? err.message : 'Failed to load more rows.'),
        data: {
          ...result.data,
          query_cursor_id: expired ? undefined : result.data.query_cursor_id,
          exhausted: expired ? true : result.data.exhausted,
        },
      })
    } finally {
      fetchingNextPageRef.current = false
    }
  }

  function handleGridScroll(e: UIEvent<HTMLDivElement>) {
    if (!canFetchMore) return
    const el = e.currentTarget
    const remaining = el.scrollHeight - el.scrollTop - el.clientHeight
    if (remaining < 400) {
      void fetchNextPage()
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
      const cell = (document.elementFromPoint(p.x, p.y) as HTMLElement | null)?.closest<HTMLElement>('[data-cell]')
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
    if (autoScrollRafRef.current == null) autoScrollRafRef.current = requestAnimationFrame(autoScrollStep)
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
    setSelection(prev => prev ? { ...prev, active: { rowIdx: ri, colIdx: ci } } : null)
  }

  function startResize(e: React.MouseEvent, colIdx: number) {
    e.preventDefault()
    resizingRef.current = { colIdx, startX: e.clientX, startWidth: colWidths[colIdx] }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    function onMouseMove(ev: MouseEvent) {
      if (!resizingRef.current) return
      const delta = ev.clientX - resizingRef.current.startX
      const newWidth = Math.max(MIN_COL_WIDTH, resizingRef.current.startWidth + delta)
      setColWidths(prev => {
        const next = [...prev]
        next[resizingRef.current!.colIdx] = newWidth
        return next
      })
    }

    function onMouseUp() {
      resizingRef.current = null
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
    }

    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
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
    setSelection({ anchor: { rowIdx: ri, colIdx: 0 }, active: { rowIdx: ri, colIdx: columns.length - 1 } })
  }

  const totalWidth = ROW_NUM_COL_WIDTH + colWidths.reduce((a, b) => a + b, 0)

  const panelValue = selection ? rows[selection.anchor.rowIdx]?.[selection.anchor.colIdx] : undefined
  const panelCol = selection ? columns[selection.anchor.colIdx] : undefined

  if (!hasColumns) {
    return (
      <div className="flex h-full min-h-0 flex-col bg-card">
        <div className="flex min-h-0 flex-1 items-center justify-center gap-2 text-xs text-muted-foreground">
          <Icon name="checkmark-circle-02" size={14} className="text-green-500" />
          <span>Query executed · {durationMs}ms</span>
        </div>
      </div>
    )
  }

  const tableEl = (
    <div ref={tableContainerRef} onKeyDown={handleTableKeyDown} className="select-none">
      <table role="grid" className="table-fixed border-separate border-spacing-0 text-xs" style={{ width: totalWidth }}>
        <thead className="sticky top-0 z-10 bg-muted/80 backdrop-blur-sm">
          <tr role="row">
            <th role="columnheader" style={{ width: ROW_NUM_COL_WIDTH }} className="sticky left-0 z-20 border-b border-r border-border bg-muted/80 px-2 py-1.5 text-right font-medium text-muted-foreground tabular-nums backdrop-blur-sm" />
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
              style={{ height: rowVirtualizer.getTotalSize() - virtualRows[virtualRows.length - 1].end }}
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
            <ResizablePanel defaultSize="25%" minSize="15%" className="flex flex-col border-l border-border">
              <CellDetailPanel
                value={panelValue}
                col={panelCol}
                tableCollapsed={tableCollapsed}
                onMaximize={() => tableCollapsed ? tablePanelRef.current?.expand() : tablePanelRef.current?.collapse()}
                onClose={closePanel}
              />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>

      <div className="flex shrink-0 items-center border-t border-border bg-muted/40 px-3 py-1 text-[10px] text-muted-foreground">
        <span className="tabular-nums">
          {rows.length === 1 ? '1 row' : `${rows.length} rows`}{queryCursorId ? ' loaded' : ''}
        </span>
        {result.isFetchingNextPage && (
          <>
            <span className="mx-1.5 opacity-40">·</span>
            <span>Loading more…</span>
          </>
        )}
        {result.cursorMessage && (
          <>
            <span className="mx-1.5 opacity-40">·</span>
            <span>{result.cursorMessage}</span>
          </>
        )}
        <span className="mx-1.5 opacity-40">·</span>
        <span className="tabular-nums">{durationMs}ms</span>
      </div>
    </div>
  )
}

// ─── Table sub-components ────────────────────────────────────────────────────

const typeColor: Record<string, string> = {
  integer: 'text-blue-500',
  decimal: 'text-blue-400',
  boolean: 'text-amber-500',
  datetime: 'text-purple-500',
  json: 'text-green-500',
  uuid: 'text-orange-400',
  bytes: 'text-red-400',
  text: 'text-muted-foreground',
}

function RowHeaderCell({ label, selected, onMouseDown, onContextMenu }: {
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
        'sticky left-0 z-[5] cursor-pointer border-b border-r border-border px-2 py-1 text-right font-mono text-muted-foreground tabular-nums',
        selected ? 'bg-primary/15' : 'bg-card',
      )}
    >
      {label}
    </td>
  )
}

function DataRow({
  row, columns, rowIdx, selection, rowSelectionMode,
  onRowHeaderMouseDown, onRowHeaderContextMenu, onCellMouseDown, onCellMouseEnter, onCellContextMenu,
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

function ColumnHeader({ col, width, onResizeStart, onContextMenu }: {
  col: ResultColumn
  width: number
  onResizeStart: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  return (
    <th
      style={{ width }}
      onContextMenu={onContextMenu}
      className="relative border-b border-r border-border px-3 py-1.5 text-left font-medium select-none overflow-hidden"
    >
      <div className="flex flex-col gap-0.5">
        <span className="truncate text-foreground">{col.name}</span>
        <span className={cn('text-[9px] font-normal uppercase tracking-wider', typeColor[col.type] ?? 'text-muted-foreground')}>
          {col.type}
        </span>
      </div>
      <div
        className="absolute inset-y-0 right-0 w-[2px] cursor-col-resize hover:bg-primary/50"
        onMouseDown={onResizeStart}
      />
    </th>
  )
}

function DataCell({ value, col, rowIdx, colIdx, isAnchor, isInRange, onMouseDown, onMouseEnter, onContextMenu }: {
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
        'max-w-0 cursor-default overflow-hidden border-b border-r border-border px-3 py-1 font-mono outline-none',
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

function CellDetailPanel({ value, col, tableCollapsed, onMaximize, onClose }: {
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
          <span className="truncate text-xs font-medium text-foreground">{col.name}</span>
          <span className={cn('shrink-0 text-[9px] font-normal uppercase tracking-wider', typeColor[col.type] ?? 'text-muted-foreground')}>
            {col.type}
          </span>
        </div>
        <div className="relative">
          {copied && (
            <div className="pointer-events-none absolute right-full top-1/2 mr-1.5 -translate-y-1/2 whitespace-nowrap rounded bg-foreground px-1.5 py-0.5 text-[10px] leading-tight text-background">
              Copied
            </div>
          )}
          <button
            type="button"
            onClick={handleCopy}
            className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label="Copy value"
          >
            <Icon name={copied ? 'tick-02' : 'copy-01'} size={13} />
          </button>
        </div>
        <button
          type="button"
          onClick={onMaximize}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label={tableCollapsed ? 'Restore' : 'Maximize'}
        >
          <Icon name={tableCollapsed ? 'minimize' : 'maximize'} size={13} />
        </button>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label="Close"
        >
          <Icon name="cancel-01" size={13} />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-3">
        <CellContent display={display} isNull={isNull} col={col} />
      </div>
    </>
  )
}

function CellContent({ display, isNull, col: _col }: { display: string; isNull: boolean; col: ResultColumn }) {
  if (isNull) {
    return <span className="font-mono text-xs italic text-muted-foreground">NULL</span>
  }
  // Future: switch on _col.type to add datetime parsed view, json pretty-print, etc.
  return (
    <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-foreground">
      {display}
    </pre>
  )
}

// ─── Value formatter ─────────────────────────────────────────────────────────

function formatValue(v: ResultValue): { display: string; isNull: boolean; isNumeric: boolean } {
  if (v.type === 'null') return { display: 'NULL', isNull: true, isNumeric: false }
  switch (v.type) {
    case 'text': return { display: v.text ?? '', isNull: false, isNumeric: false }
    case 'integer': return { display: String(v.integer ?? 0), isNull: false, isNumeric: true }
    case 'float': return { display: String(v.float ?? 0), isNull: false, isNumeric: true }
    case 'decimal': return { display: v.decimal ?? '', isNull: false, isNumeric: true }
    case 'bool': return { display: v.bool ? 'true' : 'false', isNull: false, isNumeric: false }
    case 'time': return { display: v.time ?? '', isNull: false, isNumeric: false }
    case 'bytes': return { display: '(binary)', isNull: false, isNumeric: false }
    default: return { display: '', isNull: false, isNumeric: false }
  }
}

// ─── Stub ─────────────────────────────────────────────────────────────────────

function StubPane({ message }: { message: string }) {
  return (
    <div className="flex h-full items-center justify-center bg-card">
      <p className="text-xs text-muted-foreground">{message}</p>
    </div>
  )
}
