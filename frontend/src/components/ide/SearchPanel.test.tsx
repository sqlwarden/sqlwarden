// @vitest-environment jsdom

import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace, WorkspaceFileSearchResult } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { SearchPanel } from './SearchPanel'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

afterEach(() => {
  vi.useRealTimers()
})

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 0,
  connection_count: 0,
  created_at: '',
  updated_at: '',
}

function matchResult(matchCount: number): WorkspaceFileSearchResult {
  return {
    query: 'orders',
    files_scanned: 3,
    truncated: false,
    results: [
      {
        file: {
          id: 42,
          workspace_id: 3,
          visibility: 'private',
          owner_account_id: 1,
          object_type: 'file',
          name: 'orders.sql',
          created_by: 1,
          updated_by: 1,
          created_at: '',
          updated_at: '',
        },
        path: [
          { id: 7, name: 'reports', object_type: 'folder' },
          { id: 42, name: 'orders.sql', object_type: 'file' },
        ],
        match_count: matchCount,
        snippets: [{ line: 4, column: 15, excerpt: 'select * from orders' }],
      },
    ],
  }
}

function emptyResult(): WorkspaceFileSearchResult {
  return { query: 'orders', files_scanned: 0, truncated: false, results: [] }
}

describe('SearchPanel', () => {
  let store: ReturnType<typeof createIdeStore>
  let queryClient: ReturnType<typeof createTestQueryClient>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    queryClient = createTestQueryClient()
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
      </QueryClientProvider>
    )
  }

  it('renders private results with path, match count, and snippet, and opens a match with a pending jump', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/search', () =>
        HttpResponse.json(matchResult(4)),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/search', () =>
        HttpResponse.json(emptyResult()),
      ),
    )

    render(<SearchPanel orgSlug="acme" workspace={workspace} />, { wrapper })

    const input = screen.getByPlaceholderText('Search file content...')
    act(() => {
      fireEvent.change(input, { target: { value: 'orders' } })
    })

    await waitFor(() => expect(screen.getByText('orders.sql')).toBeInTheDocument())
    expect(screen.getByText('reports')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
    // The snippet button's text is split across <span>/<mark> segments for
    // highlighting, so its accessible name (which aggregates descendant text)
    // is the right match target rather than getByText (direct text nodes only).
    const snippetButton = screen.getByRole('button', { name: 'select * from orders' })
    expect(snippetButton).toBeInTheDocument()

    fireEvent.click(snippetButton)

    await waitFor(() =>
      expect(store.getState().pendingJump).toEqual({ tabId: 'file:42', line: 4, column: 15 }),
    )
    expect(store.getState().tabs).toEqual(
      expect.arrayContaining([expect.objectContaining({ fileId: 42 })]),
    )
  })

  it('shows a hint instead of searching while the query is below the minimum length', () => {
    render(<SearchPanel orgSlug="acme" workspace={workspace} />, { wrapper })

    fireEvent.change(screen.getByPlaceholderText('Search file content...'), {
      target: { value: 'o' },
    })

    expect(screen.getByText(/at least 2 characters/i)).toBeInTheDocument()
  })
})
