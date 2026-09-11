import { useEffect, useRef, useState, type UIEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import { Icon } from '#/lib/icons'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '#/components/ui/resizable'
import { cn } from '#/lib/utils'
import type { ContextMenuItem } from '#/components/ui/context-menu'
import { useContextMenuOpener } from '#/components/ui/context-menu'
import type { ResultColumn, ResultValue } from '#/lib/api/types'
import {
  copyWithToast,
  rowToJson,
  rowToTsv,
  valuesToLines,
  writeClipboard,
} from '../contextMenus/clipboard'
import type { CellMenuCtx, ColumnHeaderMenuCtx, RowMenuCtx } from '../contextMenus/resultMenu'
import { defaultCellMenu, defaultColumnHeaderMenu, defaultRowMenu } from './defaultGridMenus'
import { nextCell } from '../resultGridNav'
import {
  cellInRange,
  formatResultValue as formatValue,
  isRowInRange,
  type CellSelection,
} from '../resultValues'
import { useColumnResize } from '../useColumnResize'
import { columnWidthFromName, distributeColumnWidths } from '../resultColumnWidths'
import { columnTypeIcon, columnTypeIconColor } from '../columnTypeIcon'
import { Tip } from '../schema-diagram/Tip'

const ROW_NUM_COL_WIDTH = 48
const MIN_COL_WIDTH = 60
const ROW_HEIGHT = 29

export type DataGridProps = {
  columns: ResultColumn[]
  rows: ResultValue[][]
  ariaLabel?: string
  emptyMessage?: string
  showColumnType?: boolean
  onScrollNearEnd?: () => void
  isLoadingMore?: boolean
  buildCellMenu?: (ctx: CellMenuCtx) => ContextMenuItem[]
  buildRowMenu?: (ctx: RowMenuCtx) => ContextMenuItem[]
  buildColumnHeaderMenu?: (ctx: ColumnHeaderMenuCtx) => ContextMenuItem[]
}

/** Virtualized, selectable, keyboard-navigable data table shared by the query
 *  results panel, the table/view "Data" section, and the CSV table view. */
export function DataGrid({
  columns,
  rows,
  ariaLabel,
  emptyMessage = 'No rows returned',
  showColumnType = true,
  onScrollNearEnd,
  isLoadingMore = false,
  buildCellMenu = defaultCellMenu,
  buildRowMenu = defaultRowMenu,
  buildColumnHeaderMenu = defaultColumnHeaderMenu,
}: DataGridProps) {
  const columnNames = columns.map((c) => c.name)
  const cellText = (v: ResultValue) => formatValue(v).display

  const columnDefaultWidths = columns.map((c) => columnWidthFromName(c.name))
  const columnShapeKey = columnNames.join('')

  const { columnWidths: colWidths, startResize } = useColumnResize(
    columns.length,
    columnDefaultWidths,
    MIN_COL_WIDTH,
    columnShapeKey || undefined,
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

  // Grid width so unresized columns can grow to fill it instead of leaving a
  // gap after the last column, for any column count (not just one).
  const [containerWidth, setContainerWidth] = useState(0)
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width
      if (width !== undefined) setContainerWidth(width)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  })
  const virtualRows = rowVirtualizer.getVirtualItems()

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
      writeClipboard(text)
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
    if (e.button !== 0) return
    e.preventDefault()
    setRowSelectionMode(true)
    setSelection({
      anchor: { rowIdx: ri, colIdx: 0 },
      active: { rowIdx: ri, colIdx: columns.length - 1 },
    })
  }

  function handleScroll(e: UIEvent<HTMLDivElement>) {
    if (!onScrollNearEnd || isLoadingMore) return
    const el = e.currentTarget
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 400) onScrollNearEnd()
  }

  const { displayWidths, columnGrew } = distributeColumnWidths(
    colWidths,
    columnDefaultWidths,
    ROW_NUM_COL_WIDTH,
    containerWidth,
  )
  const totalWidth = ROW_NUM_COL_WIDTH + displayWidths.reduce((a, b) => a + b, 0)

  const panelValue = selection
    ? rows[selection.anchor.rowIdx]?.[selection.anchor.colIdx]
    : undefined
  const panelCol = selection ? columns[selection.anchor.colIdx] : undefined

  const tableEl = (
    <div ref={tableContainerRef} onKeyDown={handleTableKeyDown} className="select-none">
      <table
        role="grid"
        aria-label={ariaLabel}
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
                width={displayWidths[i]}
                showType={showColumnType}
                onResizeStart={(e) =>
                  startResize(e, i, e.currentTarget.parentElement?.getBoundingClientRect().width)
                }
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
              // A grown numeric column left-aligns its values — right-aligned
              // short values (e.g. an id) would sit far from the row start
              // and read poorly once the column has stretched to fill space.
              columnGrew={columnGrew}
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
                {emptyMessage}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )

  return (
    // Always render the table inside ResizablePanelGroup so the table's
    // scroll container is a stable DOM element. Switching between a plain
    // <div> and a ResizablePanel on first cell click was resetting the
    // scroll position to the top because the new container starts at 0.
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
        <div
          ref={scrollRef}
          data-testid="data-grid-scroll"
          className="h-full overflow-auto"
          onScroll={handleScroll}
        >
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
              showType={showColumnType}
              tableCollapsed={tableCollapsed}
              onMaximize={() =>
                tableCollapsed ? tablePanelRef.current?.expand() : tablePanelRef.current?.collapse()
              }
              onClose={closePanel}
            />
          </ResizablePanel>
        </>
      )}
    </ResizablePanelGroup>
  )
}

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
  columnGrew,
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
  columnGrew: boolean[]
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
          grew={columnGrew[ci] ?? false}
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
  showType,
  onResizeStart,
  onContextMenu,
}: {
  col: ResultColumn
  width: number
  showType: boolean
  onResizeStart: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  const icon = columnTypeIcon(col.type)
  return (
    <th
      style={{ width }}
      onContextMenu={onContextMenu}
      // Explicit name so the resize handle's own aria-label (below) doesn't
      // leak into this header's accessible name via name-from-content.
      aria-label={col.name}
      title={showType ? `${col.name} · ${col.type}` : col.name}
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
          {showType && (
            <span className="truncate text-[9px] font-normal uppercase leading-tight tracking-wider text-muted-foreground/70">
              {col.type}
            </span>
          )}
        </div>
      </div>
      <div
        role="separator"
        aria-label={`Resize ${col.name} column`}
        aria-orientation="vertical"
        title={`Drag to resize ${col.name}`}
        className="group/resize absolute inset-y-0 right-0 w-[2px] cursor-col-resize hover:bg-primary/50"
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
  grew,
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
  grew: boolean
  isAnchor: boolean
  isInRange: boolean
  onMouseDown: (e: React.MouseEvent) => void
  onMouseEnter: () => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  const { display, isNull, isNumeric } = formatValue(value)
  // A column that grew to fill leftover space left-aligns even numeric
  // values — right-aligning a short value in a wide, auto-grown column
  // strands it far from the row start and away from its header.
  const isRightAlign = (isNumeric || col.type === 'integer' || col.type === 'decimal') && !grew

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
        isRightAlign ? 'text-right' : 'text-left',
        isNumeric || col.type === 'integer' || col.type === 'decimal' ? 'tabular-nums' : '',
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

function CellDetailPanel({
  value,
  col,
  showType,
  tableCollapsed,
  onMaximize,
  onClose,
}: {
  value: ResultValue
  col: ResultColumn
  showType: boolean
  tableCollapsed: boolean
  onMaximize: () => void
  onClose: () => void
}) {
  const [copied, setCopied] = useState(false)
  const { display, isNull } = formatValue(value)

  function handleCopy() {
    writeClipboard(isNull ? 'NULL' : display)
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
          {showType && (
            <span className="shrink-0 text-[9px] font-normal uppercase tracking-wider text-muted-foreground/70">
              {col.type}
            </span>
          )}
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
        <CellContent display={display} isNull={isNull} />
      </div>
    </>
  )
}

function CellContent({ display, isNull }: { display: string; isNull: boolean }) {
  if (isNull) {
    return <span className="text-xs italic text-muted-foreground">NULL</span>
  }
  return (
    <pre className="whitespace-pre-wrap break-words text-xs leading-relaxed text-foreground">
      {display}
    </pre>
  )
}
