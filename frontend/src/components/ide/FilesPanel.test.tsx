import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace, WorkspaceFile } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { FilesPanel } from './FilesPanel'
import { ContextMenuProvider } from '#/components/ui/context-menu'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

vi.mock('#/lib/icons', () => ({
  Icon: ({ name }: { name: string }) => <span data-icon-name={name} />,
  FileTypeIcon: ({ name }: { name: string }) => <span data-file-type-icon-name={name} />,
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
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/browser', () =>
        HttpResponse.json({
          file: null,
          path: [],
          children: sharedChildren,
        }),
      ),
    )
  }

  function renderPanel() {
    return render(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <ContextMenuProvider>
            <FilesPanel orgSlug="acme" workspace={workspace} />
          </ContextMenuProvider>
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

    await waitFor(() =>
      expect(store.getState().tabs).toEqual(
        expect.arrayContaining([expect.objectContaining({ fileId: 7, title: 'private.sql' })]),
      ),
    )
    expect(screen.getByRole('button', { name: 'team.sql' })).toBeInTheDocument()
  })

  it('loads folder children only after expansion', async () => {
    respondWith([file(9, 'Queries', 'folder')])
    renderPanel()

    expect(screen.queryByRole('button', { name: 'nested.sql' })).not.toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: 'Queries' }))

    expect(await screen.findByRole('button', { name: 'nested.sql' })).toBeInTheDocument()
  })

  it('shows file-type icons for root and nested explorer files', async () => {
    respondWith([file(7, 'query.sql'), file(8, 'results.csv'), file(9, 'Queries', 'folder')])
    renderPanel()

    const sqlRow = await screen.findByRole('button', { name: 'query.sql' })
    const csvRow = screen.getByRole('button', { name: 'results.csv' })
    expect(sqlRow.querySelector('[data-file-type-icon-name="sql"]')).toBeInTheDocument()
    expect(csvRow.querySelector('[data-file-type-icon-name="csv"]')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Queries' }))
    const nestedRow = await screen.findByRole('button', { name: 'nested.sql' })
    expect(nestedRow.querySelector('[data-file-type-icon-name="sql"]')).toBeInTheDocument()
  })

  it('offers rename and duplicate only for private explorer items', async () => {
    respondWith([file(7, 'private.sql')], [file(8, 'shared.sql')])
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const renameInput = await screen.findByLabelText('File name')
    expect(renameInput).toHaveValue('private.sql')
    fireEvent.keyDown(renameInput, { key: 'Escape' })
    expect(await screen.findByRole('button', { name: 'private.sql' })).toBeInTheDocument()

    fireEvent.contextMenu(screen.getByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Duplicate' }))
    expect(await screen.findByRole('heading', { name: 'Duplicate file' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveValue('private copy.sql'))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    fireEvent.contextMenu(screen.getByRole('button', { name: 'shared.sql' }))
    expect(screen.queryByRole('menuitem', { name: 'Rename' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Duplicate' })).not.toBeInTheDocument()
  })

  it('renames a private root file inline: stem selected, Enter submits', async () => {
    let renameBody: unknown
    let currentName = 'private.sql'
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [file(7, currentName)] }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/7', async ({ request }) => {
        renameBody = await request.json()
        currentName = 'renamed.sql'
        return HttpResponse.json(file(7, currentName))
      }),
    )
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const input = (await screen.findByLabelText('File name')) as HTMLInputElement
    await waitFor(() => {
      expect(input.selectionStart).toBe(0)
      expect(input.selectionEnd).toBe('private'.length)
    })

    fireEvent.change(input, { target: { value: 'renamed.sql' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(renameBody).toEqual({ name: 'renamed.sql' }))
    expect(await screen.findByRole('button', { name: 'renamed.sql' })).toBeInTheDocument()
    expect(screen.queryByLabelText('File name')).not.toBeInTheDocument()
  })

  it('renames a nested private file and a folder inline within the tree row', async () => {
    let rootName = 'Queries.archive'
    let nestedName = 'nested.sql'
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', ({ request }) => {
        const fileId = new URL(request.url).searchParams.get('file_id')
        const children = fileId === '9' ? [file(10, nestedName)] : [file(9, rootName, 'folder')]
        return HttpResponse.json({ file: null, path: [], children })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/10', () => {
        nestedName = 'nested-renamed.sql'
        return HttpResponse.json(file(10, nestedName))
      }),
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/9', () => {
        rootName = 'Renamed Folder'
        return HttpResponse.json(file(9, rootName, 'folder'))
      }),
    )
    renderPanel()

    fireEvent.click(await screen.findByRole('button', { name: 'Queries.archive' }))
    fireEvent.contextMenu(await screen.findByRole('button', { name: 'nested.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const nestedInput = (await screen.findByLabelText('File name')) as HTMLInputElement
    fireEvent.change(nestedInput, { target: { value: 'nested-renamed.sql' } })
    fireEvent.keyDown(nestedInput, { key: 'Enter' })
    expect(await screen.findByRole('button', { name: 'nested-renamed.sql' })).toBeInTheDocument()

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Queries.archive' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const folderInput = (await screen.findByLabelText('Folder name')) as HTMLInputElement
    expect(folderInput).toHaveValue('Queries.archive')
    expect(folderInput.selectionStart).toBe(0)
    expect(folderInput.selectionEnd).toBe('Queries.archive'.length)
    fireEvent.change(folderInput, { target: { value: 'Renamed Folder' } })
    fireEvent.keyDown(folderInput, { key: 'Enter' })
    expect(await screen.findByRole('button', { name: 'Renamed Folder' })).toBeInTheDocument()
  })

  it('cancels inline rename on Escape without calling the API', async () => {
    respondWith([file(7, 'private.sql')])
    let renameCalled = false
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/7', () => {
        renameCalled = true
        return HttpResponse.json(file(7, 'renamed.sql'))
      }),
    )
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const input = await screen.findByLabelText('File name')
    fireEvent.change(input, { target: { value: 'draft.sql' } })
    fireEvent.keyDown(input, { key: 'Escape' })

    expect(await screen.findByRole('button', { name: 'private.sql' })).toBeInTheDocument()
    expect(renameCalled).toBe(false)
  })

  it('submits a changed inline name on blur and cancels an unchanged name', async () => {
    let currentName = 'private.sql'
    let renameCalls = 0
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [file(7, currentName)] }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/7', () => {
        renameCalls += 1
        currentName = 'blurred.sql'
        return HttpResponse.json(file(7, currentName))
      }),
    )
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const changedInput = await screen.findByLabelText('File name')
    fireEvent.change(changedInput, { target: { value: 'blurred.sql' } })
    fireEvent.blur(changedInput)
    expect(await screen.findByRole('button', { name: 'blurred.sql' })).toBeInTheDocument()
    expect(renameCalls).toBe(1)

    fireEvent.contextMenu(screen.getByRole('button', { name: 'blurred.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    fireEvent.blur(await screen.findByLabelText('File name'))
    expect(await screen.findByRole('button', { name: 'blurred.sql' })).toBeInTheDocument()
    expect(renameCalls).toBe(1)
  })

  it('blocks empty inline rename submissions and shows an accessible error', async () => {
    respondWith([file(7, 'private.sql')])
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const input = await screen.findByLabelText('File name')
    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(await screen.findByRole('alert')).toHaveTextContent('Name is required.')
    expect(screen.getByLabelText('File name')).toBeInTheDocument()
  })

  it('keeps the inline editor open with the server field error on conflict', async () => {
    respondWith([file(7, 'private.sql'), file(11, 'taken.sql')])
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/files/private/7', () =>
        HttpResponse.json(
          { message: 'Invalid request.', field_errors: { name: 'Already exists.' } },
          { status: 409 },
        ),
      ),
    )
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('button', { name: 'private.sql' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const input = await screen.findByLabelText('File name')
    fireEvent.change(input, { target: { value: 'taken.sql' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(await screen.findByRole('alert')).toHaveTextContent('Already exists.')
    expect(screen.getByLabelText('File name')).toHaveValue('taken.sql')
    await waitFor(() => expect(input).toHaveFocus())

    fireEvent.change(input, { target: { value: 'taken2.sql' } })
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })
})
