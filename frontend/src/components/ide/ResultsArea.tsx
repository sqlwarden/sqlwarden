import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowExpandIcon,
  ArrowShrinkIcon,
  Cancel01Icon,
  CheckmarkCircle02Icon,
  Loading03Icon,
  TableIcon,
} from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
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
        <ResultsContent result={result} />
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

function OkState({ result }: { result: Extract<QueryResult, { status: 'ok' }> }) {
  const { durationMs } = result
  const columns = result.data.columns ?? []
  const rows = result.data.rows ?? []
  const data = { columns, rows }
  const hasColumns = columns.length > 0

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

  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      {/* Scrollable table area */}
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="min-w-max border-collapse text-xs">
          <thead className="sticky top-0 z-10 bg-muted/80 backdrop-blur-sm">
            <tr>
              <th className="w-10 border-b border-r border-border px-2 py-1.5 text-right font-medium text-muted-foreground tabular-nums select-none" />
              {data.columns.map((col, i) => (
                <ColumnHeader key={i} col={col} />
              ))}
            </tr>
          </thead>
          <tbody>
            {data.rows.map((row, ri) => (
              <tr key={ri} className="group hover:bg-accent/50">
                <td className="border-b border-r border-border px-2 py-1 text-right font-mono text-muted-foreground tabular-nums select-none">
                  {ri + 1}
                </td>
                {row.map((val, ci) => (
                  <DataCell key={ci} value={val} col={data.columns[ci]} />
                ))}
              </tr>
            ))}
            {data.rows.length === 0 && (
              <tr>
                <td
                  colSpan={data.columns.length + 1}
                  className="border-b border-border px-4 py-3 text-center text-muted-foreground"
                >
                  No rows returned
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Footer */}
      <div className="flex shrink-0 items-center border-t border-border bg-muted/40 px-3 py-1 text-[10px] text-muted-foreground">
        <span className="tabular-nums">
          {data.rows.length === 1 ? '1 row' : `${data.rows.length} rows`}
        </span>
        <span className="mx-1.5 opacity-40">·</span>
        <span className="tabular-nums">{durationMs}ms</span>
      </div>
    </div>
  )
}

// ─── Table sub-components ────────────────────────────────────────────────────

function ColumnHeader({ col }: { col: ResultColumn }) {
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
  return (
    <th className="border-b border-r border-border px-3 py-1.5 text-left font-medium">
      <div className="flex flex-col gap-0.5">
        <span className="text-foreground">{col.name}</span>
        <span className={cn('text-[9px] font-normal uppercase tracking-wider', typeColor[col.type] ?? 'text-muted-foreground')}>
          {col.type}
        </span>
      </div>
    </th>
  )
}

function DataCell({ value, col }: { value: ResultValue; col: ResultColumn }) {
  const { display, isNull, isNumeric } = formatValue(value)
  const isRightAlign = isNumeric || col.type === 'integer' || col.type === 'decimal'

  return (
    <td
      className={cn(
        'border-b border-r border-border px-3 py-1 font-mono',
        isRightAlign ? 'text-right tabular-nums' : 'text-left',
        isNull && 'text-muted-foreground/50',
      )}
    >
      {isNull ? (
        <span className="italic">NULL</span>
      ) : (
        <span className="whitespace-nowrap">{display}</span>
      )}
    </td>
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
    case 'time': return {
      display: v.time ? new Date(v.time).toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'medium' }) : '',
      isNull: false,
      isNumeric: false,
    }
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
