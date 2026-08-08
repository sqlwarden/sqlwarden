import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace, WorkspaceFile } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { saveTextAs } from './saveFile'
import { useFileActions } from './useFileActions'
import { createIdeStore, IdeStoreContext, newFileTab } from './useIdeStore'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('./saveFile', () => ({ saveTextAs: vi.fn() }))

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

function file(id: number, name = `query-${id}.sql`): WorkspaceFile {
  return {
    id,
    workspace_id: 3,
    visibility: 'private',
    owner_account_id: 1,
    object_type: 'file',
    name,
    created_by: 1,
    updated_by: 1,
    created_at: '',
    updated_at: '',
  }
}

describe('useFileActions', () => {
  let store: ReturnType<typeof createIdeStore>
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    queryClient.clear()
    store = createIdeStore('acme', 1, 'ephemeral')
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
      </QueryClientProvider>
    )
  }

  function renderActions(visibility: 'private' | 'shared' = 'private') {
    return renderHook(() => useFileActions('acme', workspace, visibility), { wrapper })
  }

  it('opens files in the active group and reports the active file', () => {
    const target = file(11)
    const { result } = renderActions()

    act(() => result.current.open(target))

    expect(store.getState().tabs).toEqual(
      expect.arrayContaining([expect.objectContaining({ fileId: 11 })]),
    )
    expect(result.current.activeFileId).toBe(11)
  })

  it('opens a file in a side editor group', () => {
    const { result } = renderActions()

    act(() => result.current.open(file(11)))
    act(() => result.current.openToSide(file(12)))

    expect(store.getState().layout[3]?.type).toBe('split')
    expect(store.getState().tabs).toEqual(
      expect.arrayContaining([expect.objectContaining({ fileId: 12 })]),
    )
  })

  it('deletes a private file, invalidates its browser tree, and closes its tab', async () => {
    const target = file(13)
    store.getState().openTab(newFileTab(target, workspace))
    server.use(
      http.delete(
        '/api/v1/orgs/acme/workspaces/3/files/private/13',
        () => new HttpResponse(null, { status: 204 }),
      ),
    )
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderActions()

    act(() => result.current.deleteFile.mutate(13))

    await waitFor(() => expect(store.getState().tabs.some((tab) => tab.fileId === 13)).toBe(false))
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['org-workspace-private-file-browser', 'acme', 3],
    })
  })

  it('downloads file content with the original filename', async () => {
    server.use(
      http.get(
        '/api/v1/orgs/acme/workspaces/3/files/private/14/content',
        () =>
          new HttpResponse('select 1', {
            headers: { ETag: '"v1"' },
          }),
      ),
    )
    const { result } = renderActions()

    await act(() => result.current.saveAs(file(14, 'one.sql')))

    expect(saveTextAs).toHaveBeenCalledWith('one.sql', 'select 1')
  })

  it('renames a file, refreshes browser and recent scopes, and updates its open tab', async () => {
    const target = file(15, 'before.sql')
    store.getState().openTab(newFileTab(target, workspace))
    let body: unknown
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/15', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(file(15, 'after.sql'))
      }),
    )
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderActions()

    act(() => result.current.renameFile.mutate({ nodeId: 15, name: 'after.sql' }))

    await waitFor(() => expect(result.current.renameFile.isSuccess).toBe(true))
    expect(body).toEqual({ name: 'after.sql' })
    expect(store.getState().tabs.find((tab) => tab.fileId === 15)?.title).toBe('after.sql')
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['org-workspace-private-file-browser', 'acme', 3],
    })
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['org-workspace-private-recent-files', 'acme', 3],
    })
    expect(toast.success).not.toHaveBeenCalled()
  })

  it('duplicates a file without opening it and supports personal workspace routes', async () => {
    let orgBody: unknown
    server.use(
      http.post(
        '/api/v1/orgs/acme/workspaces/3/files/private/16/duplicate',
        async ({ request }) => {
          orgBody = await request.json()
          return HttpResponse.json(file(17, 'source copy.sql'), { status: 201 })
        },
      ),
    )
    const orgActions = renderActions()
    act(() =>
      orgActions.result.current.duplicateFile.mutate({ nodeId: 16, name: 'source copy.sql' }),
    )
    await waitFor(() => expect(orgActions.result.current.duplicateFile.isSuccess).toBe(true))
    expect(orgBody).toEqual({ name: 'source copy.sql' })
    expect(store.getState().tabs).toHaveLength(0)

    const personalWorkspace = { ...workspace, owner_type: 'space' as const, org_id: undefined }
    let personalBody: unknown
    server.use(
      http.post('/api/v1/me/workspaces/3/files/private/16/duplicate', async ({ request }) => {
        personalBody = await request.json()
        return HttpResponse.json(file(18, 'personal copy.sql'), { status: 201 })
      }),
    )
    const personalActions = renderHook(
      () => useFileActions('ignored', personalWorkspace, 'private'),
      { wrapper },
    )
    act(() =>
      personalActions.result.current.duplicateFile.mutate({
        nodeId: 16,
        name: 'personal copy.sql',
      }),
    )
    await waitFor(() => expect(personalActions.result.current.duplicateFile.isSuccess).toBe(true))
    expect(personalBody).toEqual({ name: 'personal copy.sql' })
    expect(toast.success).not.toHaveBeenCalled()
  })

  it('refreshes the correct private or shared browser scope', () => {
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const privateActions = renderActions('private')
    act(() => privateActions.result.current.refresh())
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['org-workspace-private-file-browser', 'acme', 3],
    })

    const sharedActions = renderActions('shared')
    act(() => sharedActions.result.current.refresh())
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['org-workspace-shared-file-browser', 'acme', 3],
    })
  })
})
