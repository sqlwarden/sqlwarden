import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { JobRecord, Workspace, WorkspaceFile } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext } from '../useIdeStore'
import { rememberExportRetry } from './exportRetryCache'
import { useExportJobActions } from './useExportJobActions'
import { saveTextAs } from '../saveFile'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('../saveFile', () => ({ saveTextAs: vi.fn() }))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 0,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}

const exportedFile: WorkspaceFile = {
  id: 42,
  workspace_id: 3,
  visibility: 'private',
  owner_account_id: 1,
  object_type: 'file',
  name: 'orders.csv',
  created_by: 1,
  updated_by: 1,
  created_at: '',
  updated_at: '',
}

function job(id: string, status: JobRecord['status'] = 'succeeded'): JobRecord {
  return {
    id,
    type: 'export',
    visibility: 'private',
    status,
    run_at: '',
    priority: 0,
    attempts: 0,
    max_attempts: 1,
    created_at: '',
    updated_at: '',
    output: { file_id: 42, filename: 'orders.csv', format: 'csv', row_count: 2, byte_count: 20 },
  }
}

describe('useExportJobActions', () => {
  let store: ReturnType<typeof createIdeStore>
  let refresh: ReturnType<typeof vi.fn>
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    localStorage.clear()
    queryClient.clear()
    store = createIdeStore('acme', 1, 'ephemeral')
    refresh = vi.fn()
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
      </QueryClientProvider>
    )
  }

  function renderActions() {
    return renderHook(() => useExportJobActions('acme', workspace, refresh), { wrapper })
  }

  it('cancels an export and refreshes the job list', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/jobs/job-1/cancel', () =>
        HttpResponse.json(job('job-1', 'cancelled')),
      ),
    )
    const { result } = renderActions()

    act(() => result.current.cancel.mutate('job-1'))

    await waitFor(() => expect(refresh).toHaveBeenCalledOnce())
  })

  it('retries with the cached export request and remembers the replacement job', async () => {
    rememberExportRetry('failed-job', {
      connectionId: 7,
      sql: 'select 1',
      filename: 'one.csv',
      format: 'csv',
    })
    let body: unknown
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/exports', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(job('replacement', 'queued'))
      }),
    )
    const { result } = renderActions()

    act(() => result.current.retry.mutate('failed-job'))

    await waitFor(() => expect(refresh).toHaveBeenCalledOnce())
    expect(body).toEqual({ sql: 'select 1', format: 'csv', filename: 'one.csv' })
  })

  it('downloads the completed export using its output metadata', async () => {
    server.use(
      http.get(
        '/api/v1/orgs/acme/workspaces/3/files/private/42/content',
        () =>
          new HttpResponse('a,b\n1,2', {
            headers: { ETag: '"v1"' },
          }),
      ),
    )
    const { result } = renderActions()

    act(() => result.current.download.mutate(job('job-1')))

    await waitFor(() => expect(saveTextAs).toHaveBeenCalledWith('orders.csv', 'a,b\n1,2'))
  })

  it('opens an exported file and reveals its folder path in Files', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', () =>
        HttpResponse.json({
          file: exportedFile,
          path: [{ id: 9, name: 'Exports', object_type: 'folder' }],
          children: [],
        }),
      ),
    )
    const { result } = renderActions()

    act(() => result.current.openInEditor.mutate(job('open-job')))
    await waitFor(() => expect(store.getState().tabs.some((tab) => tab.fileId === 42)).toBe(true))

    act(() => result.current.revealInFiles.mutate(job('reveal-job')))
    await waitFor(() => expect(store.getState().activeActivityId).toBe('files'))
    expect(store.getState().expandedNodes['folder:9']).toBe(true)
  })

  it('persists dismissed jobs per workspace', () => {
    const { result } = renderActions()

    act(() => result.current.dismiss('job-1'))

    expect(result.current.dismissed.has('job-1')).toBe(true)
    expect(localStorage.getItem('sqlwarden:exports:dismissed:3')).toContain('job-1')
  })
})
