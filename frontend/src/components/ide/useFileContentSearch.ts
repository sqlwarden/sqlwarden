import { useQuery } from '@tanstack/react-query'
import {
  orgWorkspacePrivateFileSearchQueryOptions,
  orgWorkspaceSharedFileSearchQueryOptions,
} from '#/lib/api/query'
import { useDebouncedQueryText } from '#/hooks/use-debounced-query-text'

const MIN_SEARCH_QUERY_LENGTH = 2

/** Debounces a query and fires it as two parallel visibility-scoped
 *  searches, merged client-side — the same pattern FilesPanel already uses
 *  for private/shared file lists instead of one mixed-authorization call. */
export function useFileContentSearch(orgSlug: string, workspaceId: number) {
  const { searchText, setSearchText, debouncedQuery, clearSearch } = useDebouncedQueryText('', 300)
  const isQueryTooShort =
    debouncedQuery.length > 0 && debouncedQuery.length < MIN_SEARCH_QUERY_LENGTH
  const enabled = debouncedQuery.length >= MIN_SEARCH_QUERY_LENGTH

  const privateQuery = useQuery({
    ...orgWorkspacePrivateFileSearchQueryOptions(orgSlug, workspaceId, debouncedQuery),
    enabled,
  })
  const sharedQuery = useQuery({
    ...orgWorkspaceSharedFileSearchQueryOptions(orgSlug, workspaceId, debouncedQuery),
    enabled,
  })

  return {
    searchText,
    setSearchText,
    clearSearch,
    debouncedQuery,
    isQueryTooShort,
    private: privateQuery,
    shared: sharedQuery,
  }
}
