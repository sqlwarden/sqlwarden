import { useState, useRef, useEffect } from 'react'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowExpandIcon,
  ArrowShrinkIcon,
  Cancel01Icon,
  CheckmarkCircle02Icon,
  Copy01Icon,
  Loading03Icon,
  Tick02Icon,
  TableIcon,
} from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '#/components/ui/resizable'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { cn } from '#/lib/utils'
import type { ResultColumn, ResultValue } from '#/lib/api/types'
import { useIde, type QueryResult } from './useIdeStore'

export function ResultsArea() {
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const activeTabId = useIde((s) => s.activeTabId)
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
            <HugeiconsIcon icon={TableIcon} size={13} strokeWidth={2} />
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
          <HugeiconsIcon
            icon={maximizedPane === 'results' ? ArrowShrinkIcon : ArrowExpandIcon}
            size={14}
            strokeWidth={2}
          />
        </Button>
      </div>

      <TabsContent value="results" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <ResultsContent key={activeTabId} result={result} />
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

function ResultsContent({ result }: { result: QueryResult }) {
  switch (result.status) {
    case 'idle':
      return <EmptyState />
    case 'running':
      return <RunningState />
    case 'error':
      return <ErrorState message={result.message} />
    case 'ok':
      return <OkState result={result} />
  }
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
        <HugeiconsIcon icon={Loading03Icon} size={14} strokeWidth={2} className="animate-spin" />
        Running…
      </div>
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-start gap-2.5 overflow-auto p-4">
        <HugeiconsIcon icon={Cancel01Icon} size={14} strokeWidth={2} className="mt-0.5 shrink-0 text-destructive" />
        <pre className="whitespace-pre-wrap break-all font-mono text-xs text-destructive">{message}</pre>
      </div>
    </div>
  )
}

const ROW_NUM_COL_WIDTH = 48
const DEFAULT_COL_WIDTH = 150
const MIN_COL_WIDTH = 60

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

function OkState({ result }: { result: Extract<QueryResult, { status: 'ok' }> }) {
  const { durationMs } = result
  const columns = result.data.columns ?? []
  const rows = result.data.rows ?? []
  const hasColumns = columns.length > 0

  const [colWidths, setColWidths] = useState<number[]>(() => columns.map(() => DEFAULT_COL_WIDTH))
  const resizingRef = useRef<{ colIdx: number; startX: number; startWidth: number } | null>(null)

  const [selection, setSelection] = useState<CellSelection | null>(null)
  const [rowSelectionMode, setRowSelectionMode] = useState(false)
  const [tableCollapsed, setTableCollapsed] = useState(false)
  const tablePanelRef = useRef<PanelImperativeHandle>(null)
  const tableContainerRef = useRef<HTMLDivElement>(null)
  const isDraggingRef = useRef(false)

  // Stop drag on mouseup anywhere in the window.
  useEffect(() => {
    function onMouseUp() { isDraggingRef.current = false }
    window.addEventListener('mouseup', onMouseUp)
    return () => window.removeEventListener('mouseup', onMouseUp)
  }, [])

  // Focus anchor cell after keyboard navigation (skip during mouse drag).
  useEffect(() => {
    if (!selection || isDraggingRef.current) return
    tableContainerRef.current
      ?.querySelector<HTMLElement>(`[data-cell="${selection.anchor.rowIdx}-${selection.anchor.colIdx}"]`)
      ?.focus({ preventScroll: false })
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

    const { rowIdx, colIdx } = selection.anchor
    let r = rowIdx, c = colIdx
    switch (e.key) {
      case 'ArrowUp': r = Math.max(0, rowIdx - 1); break
      case 'ArrowDown': r = Math.min(rows.length - 1, rowIdx + 1); break
      case 'ArrowLeft': c = Math.max(0, colIdx - 1); break
      case 'ArrowRight': c = Math.min(columns.length - 1, colIdx + 1); break
      default: return
    }
    e.preventDefault()
    if (r !== rowIdx || c !== colIdx) {
      setSelection({ anchor: { rowIdx: r, colIdx: c }, active: { rowIdx: r, colIdx: c } })
    }
  }

  function handleCellMouseDown(ri: number, ci: number, e: React.MouseEvent) {
    e.preventDefault() // suppress browser text-selection drag (also suppresses auto-focus)
    isDraggingRef.current = true
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
          <HugeiconsIcon icon={CheckmarkCircle02Icon} size={14} strokeWidth={2} className="text-green-500" />
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
              <ColumnHeader key={i} col={col} width={colWidths[i]} onResizeStart={(e) => startResize(e, i)} />
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, ri) => (
            <tr key={ri} role="row" className="group">
              <td
                role="rowheader"
                onMouseDown={(e) => handleRowHeaderMouseDown(ri, e)}
                className={cn(
                  'sticky left-0 z-[5] cursor-pointer border-b border-r border-border px-2 py-1 text-right font-mono text-muted-foreground tabular-nums',
                  rowSelectionMode && isRowInRange(ri, selection) ? 'bg-primary/15' : 'bg-card',
                )}
              >
                {ri + 1}
              </td>
              {row.map((val, ci) => (
                <DataCell
                  key={ci}
                  value={val}
                  col={columns[ci]}
                  rowIdx={ri}
                  colIdx={ci}
                  isAnchor={selection?.anchor.rowIdx === ri && selection?.anchor.colIdx === ci}
                  isInRange={cellInRange(ri, ci, selection)}
                  onMouseDown={(e) => handleCellMouseDown(ri, ci, e)}
                  onMouseEnter={() => handleCellDragEnter(ri, ci)}
                />
              ))}
            </tr>
          ))}
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
      {selection && panelValue && panelCol ? (
        <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
          <ResizablePanel
            panelRef={tablePanelRef}
            defaultSize="75%"
            minSize="15%"
            collapsible
            collapsedSize="0%"
            className="min-h-0 overflow-auto"
            onResize={(size) => setTableCollapsed(size.asPercentage === 0)}
          >
            {tableEl}
          </ResizablePanel>
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
        </ResizablePanelGroup>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          {tableEl}
        </div>
      )}

      <div className="flex shrink-0 items-center border-t border-border bg-muted/40 px-3 py-1 text-[10px] text-muted-foreground">
        <span className="tabular-nums">
          {rows.length === 1 ? '1 row' : `${rows.length} rows`}
        </span>
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

function ColumnHeader({ col, width, onResizeStart }: {
  col: ResultColumn
  width: number
  onResizeStart: (e: React.MouseEvent) => void
}) {
  return (
    <th
      style={{ width }}
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

function DataCell({ value, col, rowIdx, colIdx, isAnchor, isInRange, onMouseDown, onMouseEnter }: {
  value: ResultValue
  col: ResultColumn
  rowIdx: number
  colIdx: number
  isAnchor: boolean
  isInRange: boolean
  onMouseDown: (e: React.MouseEvent) => void
  onMouseEnter: () => void
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
            <HugeiconsIcon icon={copied ? Tick02Icon : Copy01Icon} size={13} strokeWidth={2} />
          </button>
        </div>
        <button
          type="button"
          onClick={onMaximize}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label={tableCollapsed ? 'Restore' : 'Maximize'}
        >
          <HugeiconsIcon icon={tableCollapsed ? ArrowShrinkIcon : ArrowExpandIcon} size={13} strokeWidth={2} />
        </button>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label="Close"
        >
          <HugeiconsIcon icon={Cancel01Icon} size={13} strokeWidth={2} />
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
