import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace, WorkspaceFile } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { FilesPanel } from './FilesPanel'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

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

function file(id: number, name: string, objectType: 'file' | 'folder' = 'file'): WorkspaceFile {
  return {
    id,
    workspace_id: 3,
    visibility: 'private',
    owner_account_id: 1,
    object_type: objectType,
    name,
    created_by: 1,
    updated_by: 1,
    created_at: '',
    updated_at: '',
  }
}

describe('FilesPanel', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
  })

  function respondWith(privateChildren: WorkspaceFile[], sharedChildren: WorkspaceFile[] = []) {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', ({ request }) => {
        const fileId = new URL(request.url).searchParams.get('file_id')
        const children = fileId === '9' ? [file(10, 'nested.sql')] : privateChildren
        return HttpResponse.json({ file: null, path: [], children })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/browser', () => HttpResponse.json({
        file: null,
        path: [],
        children: sharedChildren,
      })),
    )
  }

  function renderPanel() {
    return render(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <FilesPanel orgSlug="acme" workspace={workspace} />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  it('renders independent empty states for private and shared files', async () => {
    respondWith([])
    renderPanel()

    await waitFor(() => expect(screen.getAllByText('No files yet.')).toHaveLength(2))
    expect(screen.getByText('Shared Files')).toBeInTheDocument()
  })

  it('opens a private file and preserves shared files as a separate section', async () => {
    respondWith([file(7, 'private.sql')], [file(8, 'team.sql')])
    renderPanel()

    fireEvent.click(await screen.findByRole('button', { name: 'private.sql' }))

    await waitFor(() => expect(store.getState().tabs).toEqual(expect.arrayContaining([
      expect.objectContaining({ fileId: 7, title: 'private.sql' }),
    ])))
    expect(screen.getByRole('button', { name: 'team.sql' })).toBeInTheDocument()
  })

  it('loads folder children only after expansion', async () => {
    respondWith([file(9, 'Queries', 'folder')])
    renderPanel()

    expect(screen.queryByRole('button', { name: 'nested.sql' })).not.toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: 'Queries' }))

    expect(await screen.findByRole('button', { name: 'nested.sql' })).toBeInTheDocument()
  })
})
