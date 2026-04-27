import { useEffect, useState } from 'react'
import type { ListQuery, SortOrder } from '#/lib/api/types'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'

export function useListPageState(initialQuery: ListQuery) {
  const [query, setQuery] = useState<ListQuery>(initialQuery)
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText(String(initialQuery.q ?? ''))

  useEffect(() => {
    setQuery((current) => {
      if ((current.q ?? '') === debouncedQuery) {
        return current
      }

      return {
        ...current,
        page: 1,
        q: debouncedQuery,
      }
    })
  }, [debouncedQuery])

  function setPage(page: number) {
    setQuery((current) => ({ ...current, page }))
  }

  function setPageSize(pageSize: number) {
    setQuery((current) => ({ ...current, page: 1, page_size: pageSize }))
  }

  function toggleSort(sort: string) {
    setQuery((current) => {
      const currentOrder = (current.order as SortOrder | undefined) ?? 'asc'

      return {
        ...current,
        page: 1,
        sort,
        order: current.sort === sort && currentOrder === 'asc' ? 'desc' : 'asc',
      }
    })
  }

  return {
    query,
    setQuery,
    searchText,
    setSearchText,
    clearSearch,
    setPage,
    setPageSize,
    toggleSort,
  }
}
