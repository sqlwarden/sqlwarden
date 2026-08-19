import { Icon } from '#/lib/icons'
import { Input } from '#/components/ui/input'
import { ResizablePanel, ResizablePanelGroup, ResizableHandle } from '#/components/ui/resizable'
import type {
  Workspace,
  WorkspaceFileSearchFileResult,
  WorkspaceFileSearchSnippet,
} from '#/lib/api/types'
import { SidebarPane } from './SidebarPane'
import { useFileActions } from './useFileActions'
import { useFileContentSearch } from './useFileContentSearch'
import { useIde } from './useIdeStore'
import { workspaceFileIcon } from './workspaceFileIcon'
import { highlightSnippet } from './searchSnippetHighlight'

const MIN_SEARCH_QUERY_LENGTH = 2

type SearchPanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
}

export function SearchPanel({
  orgSlug,
  workspace,
  maximized,
  onMaximizedChange,
}: SearchPanelProps) {
  const search = useFileContentSearch(orgSlug, workspace.id)
  // open()/openToSide() don't branch on the hook's visibility argument, so one
  // instance covers matches from both the private and shared result sections.
  const fileActions = useFileActions(orgSlug, workspace, 'private')
  const setPendingJump = useIde((s) => s.setPendingJump)

  // Shows the hint as soon as the user types (not debounce-delayed like the
  // hook's own isQueryTooShort, which gates the actual network requests).
  const trimmedLength = search.searchText.trim().length
  const showTooShortHint = trimmedLength > 0 && trimmedLength < MIN_SEARCH_QUERY_LENGTH

  function openMatch(result: WorkspaceFileSearchFileResult, snippet: WorkspaceFileSearchSnippet) {
    fileActions.open(result.file)
    setPendingJump({ tabId: `file:${result.file.id}`, line: snippet.line, column: snippet.column })
  }

  return (
    <SidebarPane
      title="Search"
      icon="search-01"
      maximized={maximized}
      onMaximizedChange={onMaximizedChange}
      scroll={false}
    >
      <div className="flex h-full min-h-0 flex-col">
        <div className="border-b border-border p-2">
          <Input
            value={search.searchText}
            onChange={(e) => search.setSearchText(e.target.value)}
            placeholder="Search file content..."
            className="h-7 text-xs"
            autoComplete="off"
          />
        </div>

        {showTooShortHint ? (
          <div className="px-3 py-2 text-xs text-muted-foreground">
            Type at least 2 characters to search.
          </div>
        ) : (
          <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
            <ResizablePanel defaultSize="60%" minSize="15%" className="overflow-hidden">
              <SearchResultsSection
                title="My Files"
                query={search.debouncedQuery}
                queryResult={search.private}
                onOpenMatch={openMatch}
              />
            </ResizablePanel>
            <ResizableHandle withHandle />
            <ResizablePanel defaultSize="40%" minSize="15%" className="overflow-hidden">
              <SearchResultsSection
                title="Shared"
                query={search.debouncedQuery}
                queryResult={search.shared}
                onOpenMatch={openMatch}
              />
            </ResizablePanel>
          </ResizablePanelGroup>
        )}
      </div>
    </SidebarPane>
  )
}

function SearchResultsSection({
  title,
  query,
  queryResult,
  onOpenMatch,
}: {
  title: string
  query: string
  queryResult: {
    data?: { results: WorkspaceFileSearchFileResult[] } | undefined
    isLoading: boolean
    isError: boolean
    refetch: () => void
  }
  onOpenMatch: (result: WorkspaceFileSearchFileResult, snippet: WorkspaceFileSearchSnippet) => void
}) {
  const results = queryResult.data?.results ?? []

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="border-b border-border px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground select-none">
        {title}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [scrollbar-width:thin]">
        {query.length === 0 ? (
          <div className="px-3 py-2 text-xs text-muted-foreground">
            Search this workspace's file content.
          </div>
        ) : queryResult.isLoading ? (
          <div className="px-3 py-2 text-xs text-muted-foreground">Searching...</div>
        ) : queryResult.isError ? (
          <div className="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground">
            <span>Search failed.</span>
            <button
              type="button"
              onClick={() => queryResult.refetch()}
              className="font-medium text-primary hover:underline"
            >
              Retry
            </button>
          </div>
        ) : results.length === 0 ? (
          <div className="px-3 py-2 text-xs text-muted-foreground">No matches.</div>
        ) : (
          results.map((result) => (
            <SearchResultRow
              key={result.file.id}
              result={result}
              query={query}
              onOpenMatch={onOpenMatch}
            />
          ))
        )}
      </div>
    </div>
  )
}

function SearchResultRow({
  result,
  query,
  onOpenMatch,
}: {
  result: WorkspaceFileSearchFileResult
  query: string
  onOpenMatch: (result: WorkspaceFileSearchFileResult, snippet: WorkspaceFileSearchSnippet) => void
}) {
  // result.path is the ancestor chain plus the file itself (matching Browser's
  // convention); drop the trailing file segment since the row title above
  // already names the file — showing it twice in the breadcrumb is redundant.
  const pathLabel = result.path
    .slice(0, -1)
    .map((segment) => segment.name)
    .join(' / ')

  return (
    <div className="border-b border-border/50 py-1.5">
      <div className="flex items-center gap-1.5 px-3">
        <Icon
          name={workspaceFileIcon(result.file)}
          size={13}
          className="shrink-0 text-muted-foreground"
        />
        <span className="min-w-0 flex-1 truncate text-xs font-medium" title={result.file.name}>
          {result.file.name}
        </span>
        <span className="shrink-0 text-[10px] text-muted-foreground">{result.match_count}</span>
      </div>
      {pathLabel && (
        <div className="truncate px-3 text-[10px] text-muted-foreground" title={pathLabel}>
          {pathLabel}
        </div>
      )}
      {result.snippets.map((snippet, index) => (
        <button
          key={index}
          type="button"
          onClick={() => onOpenMatch(result, snippet)}
          aria-label={snippet.excerpt}
          className="block w-full truncate px-3 py-0.5 text-left font-mono text-[11px] text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
        >
          {highlightSnippet(snippet.excerpt, query).map((segment, segmentIndex) =>
            segment.matched ? (
              <mark key={segmentIndex} className="rounded-[2px] bg-primary/20 text-foreground">
                {segment.text}
              </mark>
            ) : (
              <span key={segmentIndex}>{segment.text}</span>
            ),
          )}
        </button>
      ))}
    </div>
  )
}
