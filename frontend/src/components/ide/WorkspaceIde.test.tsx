import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '#/components/theme-provider'
import { queryKeys } from '#/lib/api/query-keys'
import type { Workspace } from '#/lib/api/types'
import { organizationRuntimeSettingsFixture, sessionFixture } from '#/test/fixtures'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { WorkspaceDocumentTitle, WorkspaceIdeContent, useWorkspaceSelection } from './WorkspaceIde'
import { createIdeStore, IdeStoreContext, type EditorTab } from './useIdeStore'
import { createYDocRegistry, YDocRegistryContext } from './useYDocRegistry'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'

const { mockIsMobile } = vi.hoisted(() => ({ mockIsMobile: vi.fn(() => false) }))
vi.mock('#/hooks/use-mobile', () => ({ useIsMobile: () => mockIsMobile() }))

const routerMocks = vi.hoisted(() => ({ navigate: vi.fn(() => Promise.resolve()) }))
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  const React = await import('react')
  return {
    ...actual,
    Link: React.forwardRef<
      HTMLAnchorElement,
      { to: string; children?: React.ReactNode } & React.AnchorHTMLAttributes<HTMLAnchorElement>
    >(({ to, children, ...props }, ref) => (
      <a ref={ref} href={to} {...props}>
        {children}
      </a>
    )),
    useNavigate: () => routerMocks.navigate,
    useSearch: () => ({}),
  }
})

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const workspaces: Workspace[] = [
  {
    id: 3,
    org_id: 1,
    owner_type: 'org',
    owner_id: 1,
    name: 'Analytics',
    environment_count: 1,
    connection_count: 1,
    created_at: '',
    updated_at: '',
  },
  {
    id: 4,
    org_id: 1,
    owner_type: 'org',
    owner_id: 1,
    name: 'Operations',
    environment_count: 0,
    connection_count: 0,
    created_at: '',
    updated_at: '',
  },
]

describe('WorkspaceIdeContent', () => {
  it('renders an accessible editor-shaped loading state', () => {
    const view = render(
      <WorkspaceIdeContent
        orgSlug="acme"
        requestedWorkspaceId={3}
        isLoading
        isError={false}
        isRetrying={false}
        workspaces={[]}
        onRetry={() => {}}
      />,
    )
    const status = screen.getByRole('status')
    expect(status).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText('Loading editor…')).toHaveClass('sr-only')
    expect(view.container.querySelectorAll('[data-slot="skeleton"]')).not.toHaveLength(0)
    expect(document.title).toBe('Editor | SQLWarden')
  })

  it('offers a retry when workspace loading fails', () => {
    const onRetry = vi.fn()
    const view = render(
      <WorkspaceIdeContent
        orgSlug="acme"
        requestedWorkspaceId={3}
        isLoading={false}
        isError
        isRetrying={false}
        workspaces={[]}
        onRetry={onRetry}
      />,
    )

    expect(screen.getByRole('heading', { name: 'Unable to load workspaces' })).toBeInTheDocument()
    expect(screen.getByText(/Check your connection and try again/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledOnce()

    view.rerender(
      <WorkspaceIdeContent
        orgSlug="acme"
        requestedWorkspaceId={3}
        isLoading={false}
        isError
        isRetrying
        workspaces={[]}
        onRetry={onRetry}
      />,
    )
    expect(screen.getByRole('button', { name: 'Retrying…' })).toBeDisabled()
  })

  it('explains how to get access when no workspace is available', () => {
    render(
      <WorkspaceIdeContent
        orgSlug="acme"
        requestedWorkspaceId={3}
        isLoading={false}
        isError={false}
        isRetrying={false}
        workspaces={[]}
        onRetry={() => {}}
      />,
    )

    expect(screen.getByRole('heading', { name: 'No workspace access' })).toBeInTheDocument()
    expect(
      screen.getByText(/Ask an organization administrator to grant you access/),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})

describe('WorkspaceDocumentTitle', () => {
  it('follows the active editor tab and workspace', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const tab = (id: string, title: string): EditorTab => ({
      id,
      title,
      workspaceId: workspaces[0].id,
      kind: 'scratch',
      content: '',
    })

    const view = render(
      <IdeStoreContext.Provider value={store}>
        <WorkspaceDocumentTitle workspace={workspaces[0]} />
      </IdeStoreContext.Provider>,
    )

    expect(document.title).toBe('Analytics | Editor | SQLWarden')

    await act(async () => store.getState().openTab(tab('first', 'Query 1')))
    await waitFor(() => expect(document.title).toBe('Query 1 | Analytics | SQLWarden'))

    await act(async () => store.getState().openTab(tab('second', 'Revenue report')))
    await waitFor(() => expect(document.title).toBe('Revenue report | Analytics | SQLWarden'))

    await act(async () => store.getState().closeTab('second'))
    await waitFor(() => expect(document.title).toBe('Query 1 | Analytics | SQLWarden'))

    await act(async () => store.getState().closeTab('first'))
    await waitFor(() => expect(document.title).toBe('Analytics | Editor | SQLWarden'))

    view.rerender(
      <IdeStoreContext.Provider value={store}>
        <WorkspaceDocumentTitle workspace={workspaces[1]} />
      </IdeStoreContext.Provider>,
    )
    expect(document.title).toBe('Operations | Editor | SQLWarden')
  })
})

describe('useWorkspaceSelection', () => {
  beforeEach(() => {
    routerMocks.navigate.mockClear()
  })

  it('follows the requested workspace and pushes a navigation on explicit selection', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    function wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }

    const { result } = renderHook(() => useWorkspaceSelection(workspaces, 3, 'acme'), { wrapper })
    expect(result.current.activeWorkspace).toEqual(workspaces[0])
    await waitFor(() => expect(store.getState().activeWorkspaceId).toBe(3))
    expect(routerMocks.navigate).not.toHaveBeenCalled()

    act(() => result.current.setActiveWorkspace(4))
    expect(routerMocks.navigate).toHaveBeenCalledWith({
      to: '/orgs/$org_slug/workspaces/$workspace_id/ide',
      params: { org_slug: 'acme', workspace_id: '4' },
    })
  })

  it('replaces the URL when the requested workspace_id is no longer accessible', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    function wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }

    const { result } = renderHook(() => useWorkspaceSelection(workspaces, 99, 'acme'), { wrapper })
    expect(result.current.activeWorkspace).toEqual(workspaces[0])
    await waitFor(() => expect(store.getState().activeWorkspaceId).toBe(3))
    await waitFor(() =>
      expect(routerMocks.navigate).toHaveBeenCalledWith({
        to: '/orgs/$org_slug/workspaces/$workspace_id/ide',
        params: { org_slug: 'acme', workspace_id: '3' },
        replace: true,
      }),
    )
  })
})

describe('WorkspaceIdeSurface responsive sidebar', () => {
  // Reaching the loaded WorkspaceIdeSurface requires the same store/registry
  // wiring the WorkspaceIde root normally provides; WorkspaceIdeContent alone
  // doesn't supply it, so this helper reconstructs it for the mounted tree.
  function renderSurface() {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const registry = createYDocRegistry(1, 'acme')
    const viewRegistry = createEditorViewRegistry()
    const queryClient = createTestQueryClient()
    queryClient.setQueryData(queryKeys.session(), sessionFixture())

    server.use(
      http.get('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json(organizationRuntimeSettingsFixture()),
      ),
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({ resource_type: 'workspace', resource_id: 3, permissions: [] }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/sessions', () =>
        HttpResponse.json({ sessions: [] }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
    )

    // Build a fresh element on every call: reusing the exact same element
    // instance across rerender() calls lets React bail out of the subtree
    // entirely (reference-identical props), which would hide the isMobile
    // change from useIsMobile()'s mock.
    const buildUi = () => (
      <ThemeProvider>
        <QueryClientProvider client={queryClient}>
          <IdeStoreContext.Provider value={store}>
            <YDocRegistryContext.Provider value={registry}>
              <EditorViewRegistryContext.Provider value={viewRegistry}>
                <WorkspaceIdeContent
                  orgSlug="acme"
                  requestedWorkspaceId={3}
                  isLoading={false}
                  isError={false}
                  isRetrying={false}
                  workspaces={workspaces}
                  onRetry={() => {}}
                />
              </EditorViewRegistryContext.Provider>
            </YDocRegistryContext.Provider>
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>
    )

    const result = render(buildUi())
    return {
      store,
      ...result,
      rerender: () => result.rerender(buildUi()),
    }
  }

  beforeEach(() => {
    mockIsMobile.mockReturnValue(true)
  })

  it('renders the Explorer sidebar inside a closed-by-default Sheet on mobile', async () => {
    const { store } = renderSurface()

    expect(await screen.findByRole('button', { name: 'Explorer' })).toBeInTheDocument()
    // The vertical editor/results split still uses a ResizablePanelGroup on
    // mobile — only the horizontal sidebar split (a vertical-orientation
    // separator bar) is replaced by the Sheet.
    const separatorOrientations = screen
      .getAllByRole('separator')
      .map((el) => el.getAttribute('aria-orientation'))
    expect(separatorOrientations).not.toContain('vertical')

    // The mobile-mount effect collapses the sidebar, which drives the Sheet
    // closed; jsdom's animation-completion path doesn't unmount the popup, so
    // assert on the store/closing state rather than DOM removal.
    await waitFor(() => expect(store.getState().sidebarCollapsed).toBe(true))
    expect(screen.getByRole('dialog')).toHaveAttribute('data-closed', '')
  })

  it('opens the Explorer drawer when the Explorer activity is clicked', async () => {
    renderSurface()

    const explorer = await screen.findByRole('button', { name: 'Explorer' })
    await userEvent.click(explorer)
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
  })

  it('keeps the resizable sidebar layout on desktop', async () => {
    mockIsMobile.mockReturnValue(false)
    renderSurface()

    expect(await screen.findByRole('button', { name: 'Explorer' })).toBeInTheDocument()
    const separatorOrientations = screen
      .getAllByRole('separator')
      .map((el) => el.getAttribute('aria-orientation'))
    expect(separatorOrientations).toContain('vertical')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('restores the persisted desktop sidebar preference after a mobile visit', async () => {
    mockIsMobile.mockReturnValue(false)
    const { store, rerender } = renderSurface()

    // Simulate a persisted desktop preference of "expanded".
    await act(async () => store.getState().setSidebarCollapsed(false))
    expect(await screen.findByRole('button', { name: 'Explorer' })).toBeInTheDocument()

    // Visit mobile: the drawer auto-closes, which writes to the same
    // persisted field the desktop panel reads.
    mockIsMobile.mockReturnValue(true)
    rerender()
    await waitFor(() => expect(store.getState().sidebarCollapsed).toBe(true))

    // Returning to desktop must restore the prior desktop preference rather
    // than leaving the sidebar collapsed.
    mockIsMobile.mockReturnValue(false)
    rerender()
    await waitFor(() => expect(store.getState().sidebarCollapsed).toBe(false))
  })
})
