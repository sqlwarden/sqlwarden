import { useRef, useState, type UIEvent } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import type { ResultValue } from '#/lib/api/types'
import {
  connectionPreviewQueryKey,
  connectionPreviewCountQueryKey,
  fetchConnectionCursorPage,
  runConnectionQuery,
} from '#/lib/api/query'
import { extractRowCount, nextCursorParam, previewColumns, previewRows, rowCountDisplay } from './previewData'
import type { ObjectViewModel } from './registry'

const PREVIEW_PAGE_SIZE = 200
// Count at most this many rows up front; beyond it the table shows `N+` with a
// button to run an exact count, so huge tables stay cheap by default.
const COUNT_THRESHOLD = 10_000

function cellText(v: ResultValue): string {
  switch (v.type) {
    case 'null': return 'NULL'
    case 'text': return v.text ?? ''
    case 'integer': return String(v.integer ?? 0)
    case 'float': return String(v.float ?? 0)
    case 'decimal': return v.decimal ?? ''
    case 'bool': return v.bool ? 'true' : 'false'
    case 'time': return v.time ?? ''
    case 'bytes': return '(binary)'
    default: return ''
  }
}

export function ObjectDataPreview({ vm }: { vm: ObjectViewModel }) {
  const { orgSlug, workspaceId, connectionId, sessionId, dialect } = vm
  const ref = vm.detail.ref
  const enabled = Boolean(sessionId)

  const data = useInfiniteQuery({
    queryKey: connectionPreviewQueryKey(orgSlug, workspaceId, connectionId, ref),
    queryFn: ({ pageParam }) =>
      pageParam === null
        ? runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, dialect.previewQuery(ref), true, PREVIEW_PAGE_SIZE)
        : fetchConnectionCursorPage(orgSlug, workspaceId, connectionId, pageParam, PREVIEW_PAGE_SIZE),
    initialPageParam: null as string | null,
    getNextPageParam: (last, _all, lastParam) => nextCursorParam(last, lastParam),
    enabled,
    staleTime: 60_000,
  })

  const boundedCount = useQuery({
    queryKey: connectionPreviewCountQueryKey(orgSlug, workspaceId, connectionId, ref),
    queryFn: () =>
      runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, dialect.boundedCountQuery(ref, COUNT_THRESHOLD + 1), false),
    enabled,
    staleTime: 60_000,
  })

  const [wantExact, setWantExact] = useState(false)
  const exactCount = useQuery({
    queryKey: [...connectionPreviewCountQueryKey(orgSlug, workspaceId, connectionId, ref), 'exact'],
    queryFn: () => runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, dialect.exactCountQuery(ref), false),
    enabled: enabled && wantExact,
    staleTime: 60_000,
  })

  const scrollRef = useRef<HTMLDivElement>(null)

  function onScroll(e: UIEvent<HTMLDivElement>) {
    if (!data.hasNextPage || data.isFetchingNextPage) return
    const el = e.currentTarget
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 400) {
      void data.fetchNextPage()
    }
  }

  if (data.isLoading) {
    return <Pane><Icon name="loading-03" size={14} className="animate-spin" /> Loading data…</Pane>
  }
  if (data.isError) {
    return <Pane className="text-destructive">{data.error instanceof Error ? data.error.message : 'Failed to load data.'}</Pane>
  }

  const pages = data.data?.pages ?? []
  const cols = previewColumns(pages)
  const rows = previewRows(pages)
  if (cols.length === 0) {
    return <Pane>No data.</Pane>
  }

  const count = rowCountDisplay({
    bounded: extractRowCount(boundedCount.data),
    exact: extractRowCount(exactCount.data),
    threshold: COUNT_THRESHOLD,
  })

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div ref={scrollRef} onScroll={onScroll} className="min-h-0 flex-1 overflow-auto">
        <table className="border-separate border-spacing-0 text-xs">
          <thead className="sticky top-0 bg-muted/80 backdrop-blur-sm">
            <tr>
              {cols.map((c) => (
                <th key={c.name} className="border-b border-r border-border px-3 py-1.5 text-left font-medium">{c.name}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, ri) => (
              <tr key={ri} className="hover:bg-accent/30">
                {row.map((v, ci) => (
                  <td key={ci} className="border-b border-r border-border px-3 py-1 font-mono">{cellText(v)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex shrink-0 items-center gap-2 border-t border-border px-3 py-1 text-[10px] text-muted-foreground">
        <span className="tabular-nums">
          {rows.length} of {count.text} {count.text === '1' ? 'row' : 'rows'}
        </span>
        {count.canShowExact && (
          <button
            type="button"
            onClick={() => setWantExact(true)}
            disabled={exactCount.isFetching}
            className="rounded border border-border px-1.5 py-0.5 text-[10px] text-foreground hover:bg-accent disabled:opacity-50"
          >
            {exactCount.isFetching ? 'Counting…' : 'Show exact count'}
          </button>
        )}
        {data.hasNextPage && (
          <button
            type="button"
            onClick={() => void data.fetchNextPage()}
            disabled={data.isFetchingNextPage}
            className="rounded border border-border px-1.5 py-0.5 text-[10px] text-foreground hover:bg-accent disabled:opacity-50"
          >
            {data.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
        )}
      </div>
    </div>
  )
}

function Pane({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`flex h-full items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}>{children}</div>
}
