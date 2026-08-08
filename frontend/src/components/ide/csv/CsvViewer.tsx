import { useDeferredValue, useEffect, useMemo, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type * as Y from 'yjs'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Textarea } from '#/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'
import { cn } from '#/lib/utils'
import { CsvParseError, parseCsv, type CsvDocument } from './parseCsv'
import { useColumnResize } from '../useColumnResize'

const ROW_NUM_COL_WIDTH = 48
const DEFAULT_COL_WIDTH = 150
const MIN_COL_WIDTH = 60
const ROW_HEIGHT = 28

type CsvViewMode = 'table' | 'raw'

/** Reads Y.Text at key 'content' and re-renders whenever it changes. */
function useYText(doc: Y.Doc): string {
  const yText = doc.getText('content')
  const [text, setText] = useState(() => yText.toString())

  useEffect(() => {
    setText(yText.toString())
    const onChange = () => setText(yText.toString())
    yText.observe(onChange)
    return () => yText.unobserve(onChange)
    // doc identity implies yText identity; re-run if doc changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doc])

  return text
}

/** Fills in generated names for missing/blank headers, e.g. "Column 3". */
function resolveHeaderNames(headers: string[], columnCount: number): string[] {
  return Array.from({ length: columnCount }, (_, i) => {
    const header = headers[i]?.trim()
    return header ? headers[i] : `Column ${i + 1}`
  })
}

type CsvViewerProps = {
  doc: Y.Doc
  className?: string
}

/** Read-only, virtualized CSV viewer for the editor group. */
export function CsvViewer({ doc, className }: CsvViewerProps) {
  const source = useYText(doc)
  const [viewMode, setViewMode] = useState<CsvViewMode>('table')
  const [search, setSearch] = useState('')
  const deferredSearch = useDeferredValue(search)

  const parsed = useMemo((): { doc: CsvDocument; error?: CsvParseError } | undefined => {
    if (viewMode === 'raw') return undefined
    try {
      return { doc: parseCsv(source) }
    } catch (e) {
      if (e instanceof CsvParseError)
        return { doc: { headers: [], rows: [], columnCount: 0 }, error: e }
      throw e
    }
  }, [source, viewMode])

  const columnNames = useMemo(
    () => (parsed ? resolveHeaderNames(parsed.doc.headers, parsed.doc.columnCount) : []),
    [parsed],
  )

  const query = deferredSearch.trim().toLowerCase()
  const filteredRows = useMemo(() => {
    if (!parsed) return []
    if (!query) return parsed.doc.rows
    return parsed.doc.rows.filter((row) => row.some((cell) => cell.toLowerCase().includes(query)))
  }, [parsed, query])

  const isEmpty = source.trim().length === 0
  const isHeaderOnly = !!parsed && !parsed.error && !isEmpty && parsed.doc.rows.length === 0
  const hasNoMatches =
    !!parsed &&
    !parsed.error &&
    query.length > 0 &&
    filteredRows.length === 0 &&
    parsed.doc.rows.length > 0

  return (
    <div className={cn('flex h-full min-h-0 flex-col bg-card', className)}>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border bg-background px-2">
        <ToggleGroup
          aria-label="CSV view"
          value={[viewMode]}
          onValueChange={(next) => {
            const selected = next[0] as CsvViewMode | undefined
            if (selected) setViewMode(selected)
          }}
          variant="outline"
          size="sm"
          spacing={0}
        >
          <ToggleGroupItem value="table" aria-label="Table view">
            Table
          </ToggleGroupItem>
          <ToggleGroupItem value="raw" aria-label="Raw view">
            Raw
          </ToggleGroupItem>
        </ToggleGroup>
        {viewMode === 'table' && (
          <div className="relative w-56 max-w-[45%] shrink">
            <Icon
              name="search-01"
              size={12}
              className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search rows…"
              aria-label="Search CSV rows"
              className="h-6 pl-6 pr-6 text-xs"
              disabled={isEmpty || !!parsed?.error}
            />
            {search && (
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                aria-label="Clear search"
                onClick={() => setSearch('')}
                className="absolute right-0.5 top-1/2 -translate-y-1/2 text-muted-foreground"
              >
                <Icon name="cancel-01" size={11} />
              </Button>
            )}
          </div>
        )}
        <div className="min-w-0 flex-1" />
        {viewMode === 'table' && parsed && !parsed.error && !isEmpty && (
          <div className="flex shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground tabular-nums">
            <span>
              {filteredRows.length === parsed.doc.rows.length
                ? `${parsed.doc.rows.length} ${parsed.doc.rows.length === 1 ? 'row' : 'rows'}`
                : `${filteredRows.length} of ${parsed.doc.rows.length} rows`}
            </span>
            <span className="opacity-40">·</span>
            <span>
              {parsed.doc.columnCount} {parsed.doc.columnCount === 1 ? 'column' : 'columns'}
            </span>
          </div>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {viewMode === 'raw' ? (
          <Textarea
            aria-label="Raw CSV"
            value={source}
            readOnly
            wrap="off"
            spellCheck={false}
            className="field-sizing-fixed h-full min-h-0 resize-none rounded-none border-0 bg-card p-3 font-mono text-xs leading-5 whitespace-pre focus-visible:border-transparent focus-visible:ring-0"
          />
        ) : parsed?.error ? (
          <CsvErrorState error={parsed.error} />
        ) : isEmpty ? (
          <CsvMessageState icon="file-01" title="This file is empty" />
        ) : isHeaderOnly ? (
          <CsvMessageState
            icon="table"
            title="No rows to show"
            description="The file only contains a header row."
          />
        ) : hasNoMatches ? (
          <CsvMessageState
            icon="search-01"
            title="No matching rows"
            description={`No rows match "${deferredSearch.trim()}".`}
          />
        ) : (
          <CsvTable columnNames={columnNames} rows={filteredRows} />
        )}
      </div>
    </div>
  )
}

function CsvErrorState({ error }: { error: CsvParseError }) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-auto p-3">
      <div className="flex items-start gap-2.5 rounded-lg border border-destructive/30 bg-destructive/5 p-3">
        <Icon name="cancel-01" size={14} className="mt-0.5 shrink-0 text-destructive" />
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-xs font-medium text-destructive">Could not parse CSV</span>
          <span className="text-xs text-destructive/90">{error.message}</span>
        </div>
      </div>
    </div>
  )
}

function CsvMessageState({
  icon,
  title,
  description,
}: {
  icon: Parameters<typeof Icon>[0]['name']
  title: string
  description?: string
}) {
  return (
    <div className="flex h-full items-center justify-center p-8 text-center">
      <div className="flex flex-col items-center gap-3">
        <div className="flex size-10 items-center justify-center rounded-lg border border-border bg-muted/50">
          <Icon name={icon} size={17} className="text-muted-foreground" />
        </div>
        <div className="flex flex-col gap-1">
          <div className="text-sm font-medium text-foreground">{title}</div>
          {description && <div className="text-xs text-muted-foreground">{description}</div>}
        </div>
      </div>
    </div>
  )
}

function CsvTable({ columnNames, rows }: { columnNames: string[]; rows: string[][] }) {
  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null)
  const { columnWidths, startResize } = useColumnResize(
    columnNames.length,
    DEFAULT_COL_WIDTH,
    MIN_COL_WIDTH,
  )

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  })
  const virtualRows = rowVirtualizer.getVirtualItems()
  const totalWidth = ROW_NUM_COL_WIDTH + columnWidths.reduce((total, width) => total + width, 0)

  return (
    <div ref={setScrollEl} className="h-full overflow-auto">
      <table
        role="grid"
        aria-label="CSV data"
        className="table-fixed border-separate border-spacing-0 text-xs"
        style={{ width: totalWidth }}
      >
        <colgroup>
          <col style={{ width: ROW_NUM_COL_WIDTH }} />
          {columnWidths.map((width, index) => (
            <col key={index} style={{ width }} />
          ))}
        </colgroup>
        <thead className="sticky top-0 z-10 bg-muted/80 backdrop-blur-sm">
          <tr role="row">
            <th
              scope="col"
              style={{ width: ROW_NUM_COL_WIDTH }}
              className="sticky left-0 z-20 border-b border-r border-border bg-muted/80 px-2 py-1.5 text-right font-medium text-muted-foreground tabular-nums backdrop-blur-sm"
            />
            {columnNames.map((name, i) => (
              <th
                key={i}
                scope="col"
                aria-label={name}
                style={{ width: columnWidths[i] }}
                title={name}
                className="relative border-b border-r border-border px-2.5 py-1.5 text-left font-medium text-foreground select-none"
              >
                <span className="block truncate">{name}</span>
                <div
                  role="separator"
                  aria-label={`Resize ${name} column`}
                  aria-orientation="vertical"
                  title={`Drag to resize ${name}`}
                  className="group/resize absolute inset-y-0 -right-1 z-20 w-2 cursor-col-resize"
                  onMouseDown={(event) => startResize(event, i)}
                >
                  <span className="absolute inset-y-0 left-1/2 w-px -translate-x-1/2 group-hover/resize:bg-primary/60" />
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {virtualRows.length > 0 && (
            <tr aria-hidden style={{ height: virtualRows[0].start }}>
              <td colSpan={columnNames.length + 1} className="p-0" />
            </tr>
          )}
          {virtualRows.map((vr) => {
            const row = rows[vr.index]
            return (
              <tr key={vr.index} role="row" style={{ height: ROW_HEIGHT }}>
                <td
                  role="rowheader"
                  className="sticky left-0 z-[5] border-b border-r border-border bg-card px-2 py-1 text-right font-mono text-muted-foreground tabular-nums"
                >
                  {vr.index + 1}
                </td>
                {columnNames.map((_, ci) => (
                  <td
                    key={ci}
                    role="gridcell"
                    title={row[ci] ?? ''}
                    className="max-w-0 overflow-hidden border-b border-r border-border px-3 py-1 font-mono text-foreground text-ellipsis whitespace-nowrap"
                  >
                    {row[ci] ?? ''}
                  </td>
                ))}
              </tr>
            )
          })}
          {virtualRows.length > 0 && (
            <tr
              aria-hidden
              style={{
                height: rowVirtualizer.getTotalSize() - virtualRows[virtualRows.length - 1].end,
              }}
            >
              <td colSpan={columnNames.length + 1} className="p-0" />
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}
