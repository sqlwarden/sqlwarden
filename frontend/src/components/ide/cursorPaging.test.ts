import { describe, expect, it } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import type { ResultSet } from '#/lib/api/types'
import {
  applyCursorPageError,
  mergeCursorPage,
  resultRowCountLabel,
  type SuccessfulQueryResult,
} from './cursorPaging'

const initial: SuccessfulQueryResult = {
  status: 'ok',
  durationMs: 5,
  sql: 'select * from users',
  connectionId: 9,
  isFetchingNextPage: true,
  data: {
    columns: [],
    rows: [[{ type: 'integer', integer: 1 }]],
    duration_ms: 5,
    truncated: false,
    rows_returned: 1,
    bytes_returned: 10,
    query_cursor_id: 'cursor-1',
    exhausted: false,
    page_size: 20,
    transaction: { mode: 'auto', open: false, pending_statements: 0, statements: [] },
  },
}

describe('mergeCursorPage', () => {
  it('merges rows and cumulative result metadata while retaining an active cursor', () => {
    const page: ResultSet = {
      columns: [],
      rows: [[{ type: 'integer', integer: 2 }]],
      duration_ms: 2,
      truncated: true,
      truncation_reason: 'page_limit',
      rows_returned: 1,
      bytes_returned: 15,
      exhausted: false,
      transaction: { mode: 'auto', open: false, pending_statements: 0, statements: [] },
    }

    const merged = mergeCursorPage(initial, page, 'cursor-1')
    expect(merged.data.rows).toHaveLength(2)
    expect(merged.data.rows_returned).toBe(2)
    expect(merged.data.bytes_returned).toBe(25)
    expect(merged.data.truncated).toBe(true)
    expect(merged.data.truncation_reason).toBe('page_limit')
    expect(merged.data.query_cursor_id).toBe('cursor-1')
    expect(merged.data.exhausted).toBe(false)
    expect(merged.isFetchingNextPage).toBe(false)
    expect(merged.durationMs).toBe(7)
    expect(merged.data.duration_ms).toBe(7)
    expect(merged.lastPageDurationMs).toBe(2)
  })

  it('removes the cursor when the page exhausts it', () => {
    const merged = mergeCursorPage(
      initial,
      {
        ...initial.data,
        rows: [],
        rows_returned: 0,
        bytes_returned: 0,
        exhausted: true,
      },
      'cursor-1',
    )
    expect(merged.data.query_cursor_id).toBeUndefined()
    expect(merged.data.exhausted).toBe(true)
  })
})

describe('resultRowCountLabel', () => {
  it('shows a plain count when exhaustion is unknown', () => {
    expect(resultRowCountLabel(5, undefined)).toBe('5 rows')
    expect(resultRowCountLabel(1, undefined)).toBe('1 row')
  })

  it('marks the count as complete once the cursor is exhausted', () => {
    expect(resultRowCountLabel(5, true)).toBe('All 5 rows fetched')
    expect(resultRowCountLabel(1, true)).toBe('All 1 row fetched')
  })

  it('marks the count as partial while more rows remain', () => {
    expect(resultRowCountLabel(5, false)).toBe('Fetched 5 rows')
  })
})

describe('applyCursorPageError', () => {
  it('retires an expired cursor', () => {
    const next = applyCursorPageError(
      initial,
      new ApiError('gone', 410, {
        code: 'query_cursor_unavailable',
      }),
    )
    expect(next.cursorMessage).toBe('Cursor expired. Run the query again.')
    expect(next.data.query_cursor_id).toBeUndefined()
    expect(next.data.exhausted).toBe(true)
  })

  it('keeps a recoverable cursor after a transient failure', () => {
    const next = applyCursorPageError(initial, new Error('Network unavailable.'))
    expect(next.cursorMessage).toBe('Network unavailable.')
    expect(next.data.query_cursor_id).toBe('cursor-1')
    expect(next.data.exhausted).toBe(false)
  })
})
