import type { EditorTab } from './useIdeStore'

/** Whether `tab` holds runnable SQL — a console/scratch tab, or a .sql file —
 *  as opposed to a database object/diagram tab or a non-SQL file. */
export function isSqlEditorTab(tab: EditorTab | undefined): boolean {
  return (
    !!tab &&
    (tab.kind === 'scratch' ||
      tab.kind === 'connection' ||
      (tab.kind === 'file' && tab.title.toLowerCase().endsWith('.sql')))
  )
}

/**
 * Splits `text` into statement spans on semicolons that are not inside string
 * literals or comments. Spans are [inclusive_start, exclusive_end] and each
 * next span starts at the first non-whitespace char after its semicolon, so
 * the gap between statements belongs to no span. Returns a single span
 * covering the whole trimmed text when no semicolons are present.
 */
function sqlStatementSpans(text: string): Array<[number, number]> {
  type LexState = 'normal' | 'sq' | 'dq' | 'lc' | 'bc'
  let state: LexState = 'normal'
  const semis: number[] = []

  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    const n = text[i + 1] ?? ''
    switch (state) {
      case 'normal':
        if (c === "'") {
          state = 'sq'
        } else if (c === '"') {
          state = 'dq'
        } else if (c === '-' && n === '-') {
          state = 'lc'
          i++
        } else if (c === '/' && n === '*') {
          state = 'bc'
          i++
        } else if (c === ';') {
          semis.push(i)
        }
        break
      case 'sq':
        if (c === "'" && n === "'") {
          i++
        } // escaped ''
        else if (c === "'") {
          state = 'normal'
        }
        break
      case 'dq':
        if (c === '"' && n === '"') {
          i++
        } // escaped ""
        else if (c === '"') {
          state = 'normal'
        }
        break
      case 'lc':
        if (c === '\n') {
          state = 'normal'
        }
        break
      case 'bc':
        if (c === '*' && n === '/') {
          state = 'normal'
          i++
        }
        break
    }
  }

  if (semis.length === 0) return [[0, text.length]]

  const spans: Array<[number, number]> = []
  let start = 0
  for (const semi of semis) {
    spans.push([start, semi + 1]) // include the semicolon in the span
    let nextStart = semi + 1
    while (nextStart < text.length && /\s/.test(text[nextStart])) nextStart++
    start = nextStart
  }
  // Trailing content after the last semicolon (statement without terminator).
  if (text.slice(start).trim()) {
    spans.push([start, text.length])
  }
  return spans
}

/**
 * Returns the SQL statement that contains `cursor` (a character offset).
 * When the cursor sits in whitespace between statements, the preceding
 * statement wins.
 */
export function sqlStatementAtCursor(text: string, cursor: number): string {
  const spans = sqlStatementSpans(text)

  // Find the span that contains the cursor.
  for (const [s, e] of spans) {
    if (cursor >= s && cursor < e) return text.slice(s, e).trim()
  }

  // Cursor is in trailing whitespace — return the last span that started at or
  // before the cursor (i.e. the statement immediately preceding it).
  let best = spans[0]
  for (const span of spans) {
    if (span[0] <= cursor) best = span
  }
  return text.slice(best[0], best[1]).trim()
}

/** Counts non-empty SQL statements in `text`, splitting on top-level semicolons. */
export function countSqlStatements(text: string): number {
  return sqlStatementSpans(text).filter(([s, e]) => text.slice(s, e).trim().length > 0).length
}

/** Splits `text` into its non-empty top-level statements, in order, stripping each statement's terminating semicolon. */
export function splitSqlStatements(text: string): string[] {
  return sqlStatementSpans(text)
    .map(([start, end]) => text.slice(start, end).trim())
    .map((statement) => (statement.endsWith(';') ? statement.slice(0, -1).trim() : statement))
    .filter((statement) => statement.length > 0)
}

/** One non-empty top-level statement plus its byte offsets into the original,
 *  untrimmed text — for callers that need to map back to editor positions
 *  (e.g. to place UI over a specific statement). Unlike `splitSqlStatements`,
 *  the terminating semicolon is kept in both `sql` and the offset range. */
export type SqlStatementWithOffsets = { sql: string; start: number; end: number }

/** Same statement boundaries as `splitSqlStatements`, with each statement's
 *  offsets into `text`. Leading/trailing whitespace within a span is excluded
 *  from the offsets, matching the trimmed `sql`. */
export function sqlStatementsWithOffsets(text: string): SqlStatementWithOffsets[] {
  const result: SqlStatementWithOffsets[] = []
  for (const [start, end] of sqlStatementSpans(text)) {
    const raw = text.slice(start, end)
    const sql = raw.trim()
    const withoutTerminator = sql.endsWith(';') ? sql.slice(0, -1).trim() : sql
    if (!withoutTerminator) continue
    const leading = raw.indexOf(sql)
    result.push({ sql, start: start + leading, end: start + leading + sql.length })
  }
  return result
}
