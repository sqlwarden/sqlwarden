import { describe, it, expect } from 'vitest'
import { nextCursorParam, previewColumns, previewRows, extractRowCount, rowCountDisplay } from './previewData'
import type { ResultSet } from '#/lib/api/types'

const page = (over: Partial<ResultSet>): ResultSet => ({
  columns: null, rows: null, duration_ms: 0, truncated: false, rows_returned: 0, bytes_returned: 0, ...over,
})

describe('nextCursorParam', () => {
  it('returns the cursor id while not exhausted', () => {
    expect(nextCursorParam(page({ exhausted: false, query_cursor_id: 'c2' }), 'c1')).toBe('c2')
  })
  it('falls back to the previous param when the id is missing', () => {
    expect(nextCursorParam(page({ exhausted: false }), 'c1')).toBe('c1')
  })
  it('returns undefined once exhausted', () => {
    expect(nextCursorParam(page({ exhausted: true, query_cursor_id: 'c2' }), 'c1')).toBeUndefined()
  })
  it('returns undefined when exhausted is absent (non-cursor result)', () => {
    expect(nextCursorParam(page({}), 'c1')).toBeUndefined()
  })
})

describe('previewColumns / previewRows', () => {
  it('takes columns from the first page and concatenates rows across pages', () => {
    const pages: ResultSet[] = [
      page({ columns: [{ name: 'id', type: 'integer', raw_type: 'int', nullable: false }], rows: [[{ type: 'integer', integer: 1 }]] }),
      page({ columns: [{ name: 'id', type: 'integer', raw_type: 'int', nullable: false }], rows: [[{ type: 'integer', integer: 2 }]] }),
    ]
    expect(previewColumns(pages).map((c) => c.name)).toEqual(['id'])
    expect(previewRows(pages)).toHaveLength(2)
  })
})

describe('rowCountDisplay', () => {
  const threshold = 1000
  it('shows … while the bounded count is loading', () => {
    expect(rowCountDisplay({ bounded: null, exact: null, threshold })).toEqual({ text: '…', canShowExact: false })
  })
  it('shows the exact bounded count when under the threshold', () => {
    expect(rowCountDisplay({ bounded: 42, exact: null, threshold })).toEqual({ text: '42', canShowExact: false })
  })
  it('shows <threshold>+ and offers exact when the cap is hit', () => {
    expect(rowCountDisplay({ bounded: 1001, exact: null, threshold })).toEqual({ text: '1000+', canShowExact: true })
  })
  it('prefers an available exact count over the bounded one', () => {
    expect(rowCountDisplay({ bounded: 1001, exact: 5123, threshold })).toEqual({ text: '5123', canShowExact: false })
  })
})

describe('extractRowCount', () => {
  it('reads an integer count', () => {
    expect(extractRowCount(page({ rows: [[{ type: 'integer', integer: 42 }]] }))).toBe(42)
  })
  it('reads zero when the integer field is omitted (json omitempty)', () => {
    expect(extractRowCount(page({ rows: [[{ type: 'integer' }]] }))).toBe(0)
  })
  it('reads a decimal count (e.g. bigint as string)', () => {
    expect(extractRowCount(page({ rows: [[{ type: 'decimal', decimal: '123' }]] }))).toBe(123)
  })
  it('returns null when there is no row', () => {
    expect(extractRowCount(page({ rows: [] }))).toBeNull()
    expect(extractRowCount(undefined)).toBeNull()
  })
})
