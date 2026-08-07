import type { PropsWithChildren } from 'react'
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { WorkspaceIdeContent, useWorkspaceSelection } from './WorkspaceIde'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

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
  })

  it('offers a retry when workspace loading fails', () => {
    const onRetry = vi.fn()
    const view = render(
      <WorkspaceIdeContent
        orgSlug="acme"
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

describe('useWorkspaceSelection', () => {
  it('selects the first accessible workspace and follows explicit selection', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    function wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }

    const { result } = renderHook(() => useWorkspaceSelection(workspaces), { wrapper })
    expect(result.current.activeWorkspace).toEqual(workspaces[0])
    await waitFor(() => expect(store.getState().activeWorkspaceId).toBe(3))

    await act(async () => result.current.setActiveWorkspace(4))
    expect(result.current.activeWorkspace).toEqual(workspaces[1])
  })

  it('recovers when a persisted workspace is no longer accessible', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    store.getState().setActiveWorkspace(99)
    function wrapper({ children }: PropsWithChildren) {
      return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
    }

    const { result } = renderHook(() => useWorkspaceSelection(workspaces), { wrapper })
    expect(result.current.activeWorkspace).toEqual(workspaces[0])
    await waitFor(() => expect(store.getState().activeWorkspaceId).toBe(3))
  })
})
