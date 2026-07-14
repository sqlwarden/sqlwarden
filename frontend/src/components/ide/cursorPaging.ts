import { ApiError, errorMessage } from '#/lib/api/errors'
import type { ResultSet } from '#/lib/api/types'
import type { QueryResult } from './useIdeStore'

export type SuccessfulQueryResult = Extract<QueryResult, { status: 'ok' }>

export function mergeCursorPage(
  result: SuccessfulQueryResult,
  page: ResultSet,
  currentCursorId: string,
): SuccessfulQueryResult {
  const rows = [...(result.data.rows ?? []), ...(page.rows ?? [])]
  const exhausted = page.exhausted ?? true

  return {
    ...result,
    isFetchingNextPage: false,
    cursorMessage: undefined,
    data: {
      ...result.data,
      rows,
      rows_returned: rows.length,
      bytes_returned: result.data.bytes_returned + page.bytes_returned,
      truncated: result.data.truncated || page.truncated,
      truncation_reason: page.truncation_reason ?? result.data.truncation_reason,
      query_cursor_id: exhausted ? undefined : (page.query_cursor_id ?? currentCursorId),
      exhausted,
      page_size: page.page_size ?? result.data.page_size,
    },
  }
}

export function applyCursorPageError(
  result: SuccessfulQueryResult,
  error: unknown,
): SuccessfulQueryResult {
  const expired = error instanceof ApiError && error.code === 'query_cursor_unavailable'
  return {
    ...result,
    isFetchingNextPage: false,
    cursorMessage: expired
      ? 'Cursor expired. Run the query again.'
      : errorMessage(error, 'Failed to load more rows.'),
    data: {
      ...result.data,
      query_cursor_id: expired ? undefined : result.data.query_cursor_id,
      exhausted: expired ? true : result.data.exhausted,
    },
  }
}
