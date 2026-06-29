import type { ResultColumn, ResultRow, ResultSet, ResultValue } from '#/lib/api/types'

/** The next cursor id to fetch, or undefined when the stream is exhausted.
 *  Falls back to the previous page param when a fetch page omits the id but is
 *  not yet exhausted (mirrors the result grid's cursor handling). */
export function nextCursorParam(last: ResultSet, lastParam: string | null): string | undefined {
  if (last.exhausted !== false) return undefined
  return last.query_cursor_id ?? lastParam ?? undefined
}

/** Columns come from the first page; later cursor pages repeat them. */
export function previewColumns(pages: ResultSet[]): ResultColumn[] {
  return pages[0]?.columns ?? []
}

export function previewRows(pages: ResultSet[]): ResultRow[] {
  return pages.flatMap((p) => p.rows ?? [])
}

export interface CountDisplay {
  /** Human label for the total: a number, `<threshold>+`, or `…` while loading. */
  text: string
  /** True when the bounded count hit its cap and an exact count can still be run. */
  canShowExact: boolean
}

/** Resolves what to show for the total row count. An exact value always wins;
 *  otherwise a bounded count that reached `threshold + 1` is shown as
 *  `<threshold>+` with the option to fetch the exact number. */
export function rowCountDisplay({ bounded, exact, threshold }: { bounded: number | null; exact: number | null; threshold: number }): CountDisplay {
  if (exact != null) return { text: String(exact), canShowExact: false }
  if (bounded == null) return { text: '…', canShowExact: false }
  if (bounded > threshold) return { text: `${threshold}+`, canShowExact: true }
  return { text: String(bounded), canShowExact: false }
}

/** Reads a single integer out of a `SELECT COUNT(*)` result, or null if absent.
 *  A zero integer omits its `integer` field on the wire (json omitempty), so a
 *  present integer value with no number means 0, not "missing". */
export function extractRowCount(rs: ResultSet | undefined): number | null {
  const v: ResultValue | undefined = rs?.rows?.[0]?.[0]
  if (!v) return null
  if (v.type === 'integer') return v.integer ?? 0
  if (v.type === 'decimal' && v.decimal != null) {
    const n = Number(v.decimal)
    return Number.isFinite(n) ? n : null
  }
  return null
}
