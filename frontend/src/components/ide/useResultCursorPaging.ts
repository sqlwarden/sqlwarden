import { useRef, type UIEvent } from 'react'
import { fetchConnectionCursorPage } from '#/lib/api/query'
import { applyCursorPageError, mergeCursorPage } from './cursorPaging'
import { useIde, type QueryResult } from './useIdeStore'

type SuccessfulQueryResult = Extract<QueryResult, { status: 'ok' }>

type ResultCursorPagingOptions = {
  activeTabId?: string
  connectionId?: number
  index: number
  orgSlug: string
  result: SuccessfulQueryResult
  workspaceId: number
}

export function useResultCursorPaging({
  activeTabId,
  connectionId,
  index,
  orgSlug,
  result,
  workspaceId,
}: ResultCursorPagingOptions) {
  const fetchingRef = useRef(false)
  const setStatementResult = useIde((state) => state.setStatementResult)
  const cursorId = result.data.query_cursor_id
  const canFetchMore = Boolean(
    activeTabId &&
    connectionId &&
    cursorId &&
    result.data.exhausted === false &&
    !result.isFetchingNextPage,
  )

  async function fetchNextPage() {
    if (!activeTabId || !connectionId || !cursorId || !canFetchMore || fetchingRef.current) {
      return
    }

    fetchingRef.current = true
    setStatementResult(activeTabId, index, {
      ...result,
      isFetchingNextPage: true,
      cursorMessage: undefined,
    })

    try {
      const page = await fetchConnectionCursorPage(
        orgSlug,
        workspaceId,
        connectionId,
        cursorId,
        result.data.page_size,
      )
      setStatementResult(activeTabId, index, mergeCursorPage(result, page, cursorId))
    } catch (error) {
      setStatementResult(activeTabId, index, applyCursorPageError(result, error))
    } finally {
      fetchingRef.current = false
    }
  }

  function handleGridScroll(event: UIEvent<HTMLDivElement>) {
    if (!canFetchMore) {
      return
    }

    const element = event.currentTarget
    const remaining = element.scrollHeight - element.scrollTop - element.clientHeight
    if (remaining < 400) {
      void fetchNextPage()
    }
  }

  return {
    canFetchMore,
    fetchNextPage,
    handleGridScroll,
  }
}
