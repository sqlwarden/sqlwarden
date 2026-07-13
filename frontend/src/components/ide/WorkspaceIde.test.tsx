import type { PropsWithChildren } from 'react'
import { act, render, renderHook, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { WorkspaceIdeContent, useWorkspaceSelection } from './WorkspaceIde'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

const workspaces: Workspace[] = [
  {
    id: 3, org_id: 1, owner_type: 'org', owner_id: 1, name: 'Analytics',
    environment_count: 1, connection_count: 1, created_at: '', updated_at: '',
  },
  {
    id: 4, org_id: 1, owner_type: 'org', owner_id: 1, name: 'Operations',
    environment_count: 0, connection_count: 0, created_at: '', updated_at: '',
  },
]

describe('WorkspaceIdeContent', () => {
  it('renders stable loading, error, and empty states', () => {
    const view = render(<WorkspaceIdeContent orgSlug="acme" isLoading isError={false} workspaces={[]} />)
    expect(screen.getByText('Loading workspaces…')).toBeInTheDocument()

    view.rerender(<WorkspaceIdeContent orgSlug="acme" isLoading={false} isError workspaces={[]} />)
    expect(screen.getByText('Unable to load workspaces.')).toBeInTheDocument()

    view.rerender(<WorkspaceIdeContent orgSlug="acme" isLoading={false} isError={false} workspaces={[]} />)
    expect(screen.getByText('No accessible workspaces.')).toBeInTheDocument()
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
