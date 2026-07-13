import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { WorkspaceFile } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { CreateItemDialog } from './CreateItemDialog'
import { SaveAsDialog } from './SaveAsDialog'
import type { EditorTab } from './useIdeStore'

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

function provider(children: React.ReactNode) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>
}

describe('workspace file dialogs', () => {
  it('creates a file, initializes its content, and returns the created item', async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    const onOpenChange = vi.fn()
    let createBody: unknown
    let initialized = false
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/files/private', async ({ request }) => {
        createBody = await request.json()
        return HttpResponse.json(file(7, 'report.sql'), { status: 201 })
      }),
      http.put('/api/v1/orgs/acme/workspaces/3/files/private/7/content', () => {
        initialized = true
        return new HttpResponse(null, { headers: { ETag: '"v1"' } })
      }),
    )
    render(provider(
      <CreateItemDialog open onOpenChange={onOpenChange} kind="file" parentId={9} orgSlug="acme" workspaceId={3} onSuccess={onSuccess} />,
    ))

    const input = screen.getByLabelText('Name')
    await user.clear(input)
    await user.type(input, 'report')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith(expect.objectContaining({ id: 7 })))
    expect(createBody).toEqual(expect.objectContaining({ name: 'report.sql', object_type: 'file', parent_id: 9 }))
    expect(initialized).toBe(true)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('creates folders without a content request', async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    let contentRequests = 0
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/files/private', () => HttpResponse.json(file(8, 'Queries', 'folder'), { status: 201 })),
      http.put('/api/v1/orgs/acme/workspaces/3/files/private/8/content', () => {
        contentRequests += 1
        return new HttpResponse(null)
      }),
    )
    render(provider(
      <CreateItemDialog open onOpenChange={vi.fn()} kind="folder" parentId={null} orgSlug="acme" workspaceId={3} onSuccess={onSuccess} />,
    ))

    await user.type(screen.getByLabelText('Name'), 'Queries')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
    expect(contentRequests).toBe(0)
  })

  it('saves editor content into a selected workspace folder', async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    const folder = file(9, 'Reports', 'folder')
    let createBody: unknown
    let savedContent = ''
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/browser', () => HttpResponse.json({ file: null, path: [], children: [folder] })),
      http.post('/api/v1/orgs/acme/workspaces/3/files/private', async ({ request }) => {
        createBody = await request.json()
        return HttpResponse.json(file(10, 'daily.sql'), { status: 201 })
      }),
      http.put('/api/v1/orgs/acme/workspaces/3/files/private/10/content', async ({ request }) => {
        savedContent = await request.text()
        return new HttpResponse(null, { headers: { ETag: '"etag-10"' } })
      }),
    )
    const tab: EditorTab = { id: 'scratch:1', workspaceId: 3, title: 'Console', kind: 'scratch', content: 'select 42' }
    render(provider(
      <SaveAsDialog open onOpenChange={vi.fn()} tab={tab} orgSlug="acme" workspaceId={3} onSuccess={onSuccess} />,
    ))

    await user.click(await screen.findByRole('button', { name: /Reports/ }))
    const input = screen.getByLabelText('Name')
    await user.clear(input)
    await user.type(input, 'daily')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith(expect.objectContaining({ id: 10 }), 'etag-10'))
    expect(createBody).toEqual(expect.objectContaining({ name: 'daily.sql', parent_id: 9 }))
    expect(savedContent).toBe('select 42')
  })
})
