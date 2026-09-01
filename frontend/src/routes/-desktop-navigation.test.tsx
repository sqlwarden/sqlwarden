import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { permission } from '#/lib/permissions'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { sessionHandler, setupStatusHandler } from '#/test/handlers'
import {
  desktopCapabilitiesFixture,
  organizationFixture,
  organizationRuntimeSettingsFixture,
  sessionFixture,
} from '#/test/fixtures'

const { mockIDBGet } = vi.hoisted(() => ({ mockIDBGet: vi.fn(async () => undefined as unknown) }))

vi.mock('idb-keyval', () => ({
  get: mockIDBGet,
  set: vi.fn(async () => undefined),
  del: vi.fn(async () => undefined),
}))

vi.mock('#/lib/icons', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#/lib/icons')>()
  return {
    ...actual,
    Icon: ({ name }: { name: string }) => <span data-icon-name={name} />,
  }
})

const organization = organizationFixture({ id: 1, slug: 'local', name: 'Local' })
const workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  description: 'Desktop workspace',
  environment_count: 2,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}
const secondWorkspace = { ...workspace, id: 4, name: 'Operations' }
const permissions = Object.values(permission)

describe('desktop organization navigation', () => {
  beforeEach(() => {
    mockIDBGet.mockReset()
    mockIDBGet.mockResolvedValue(undefined)
    setAccessToken('desktop-token')
    server.use(
      setupStatusHandler({
        configured: true,
        mode: 'desktop',
        capabilities: desktopCapabilitiesFixture(),
      }),
      sessionHandler(
        sessionFixture({ organizations: [organization], personal_spaces_enabled: true }),
      ),
      http.get('/api/v1/orgs/local', () => HttpResponse.json(organization)),
      http.get('/api/v1/orgs/local/runtime-settings', () =>
        HttpResponse.json(organizationRuntimeSettingsFixture()),
      ),
      http.get('/api/v1/orgs/local/workspaces', () =>
        HttpResponse.json({ items: [workspace], page: 1, page_size: 100, total: 1 }),
      ),
      http.get('/api/v1/orgs/local/workspaces/3', () => HttpResponse.json(workspace)),
      http.get('/api/v1/orgs/local/workspaces/3/environments', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.get('/api/v1/orgs/local/workspaces/3/connections', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.get('/api/v1/orgs/local/workspaces/3/files/private/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.get('/api/v1/orgs/local/workspaces/3/files/shared/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.get('/api/v1/orgs/local/workspaces/3/sessions', () =>
        HttpResponse.json({ sessions: [] }),
      ),
      http.get('/api/v1/orgs/local/permissions/effective', ({ request }) => {
        const url = new URL(request.url)
        return HttpResponse.json({
          resource_type: url.searchParams.get('resource_type') ?? 'org',
          resource_id: Number(url.searchParams.get('resource_id') ?? 1),
          permissions,
        })
      }),
    )
  })

  it('never renders the web landing hub and enters the only desktop workspace', async () => {
    const { router } = renderRoute('/')

    await waitFor(() => expect(router.state.location.pathname).toBe('/orgs/local/workspaces/3/ide'))
    expect(screen.queryByText('Choose where to continue')).not.toBeInTheDocument()
    expect(screen.queryByText('Personal Workspace')).not.toBeInTheDocument()
    expect(screen.queryByText('Administration')).not.toBeInTheDocument()
  })

  it('uses the last active workspace when more than one desktop workspace exists', async () => {
    mockIDBGet.mockResolvedValue(JSON.stringify({ state: { activeWorkspaceId: 4 } }))
    server.use(
      http.get('/api/v1/orgs/local/workspaces', () =>
        HttpResponse.json({
          items: [workspace, secondWorkspace],
          page: 1,
          page_size: 100,
          total: 2,
        }),
      ),
      http.get('/api/v1/orgs/local/workspaces/4', () => HttpResponse.json(secondWorkspace)),
      http.get('/api/v1/orgs/local/workspaces/4/environments', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.get('/api/v1/orgs/local/workspaces/4/connections', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.get('/api/v1/orgs/local/workspaces/4/files/private/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.get('/api/v1/orgs/local/workspaces/4/files/shared/browser', () =>
        HttpResponse.json({ file: null, path: [], children: [] }),
      ),
      http.get('/api/v1/orgs/local/workspaces/4/sessions', () =>
        HttpResponse.json({ sessions: [] }),
      ),
    )

    const { router } = renderRoute('/')
    await waitFor(() => expect(router.state.location.pathname).toBe('/orgs/local/workspaces/4/ide'))
  })

  it('redirects the desktop organization workspace list into the editor', async () => {
    const { router } = renderRoute('/orgs/local/workspaces')

    await waitFor(() => expect(router.state.location.pathname).toBe('/orgs/local/workspaces/3/ide'))
    expect(screen.queryByRole('heading', { name: 'Workspaces' })).not.toBeInTheDocument()
  })

  it('hides access-control overview cards and preserves workspace context in the editor link', async () => {
    const { router, user } = renderRoute('/orgs/local/workspaces/3')

    expect(await screen.findByRole('heading', { name: 'Analytics' })).toBeInTheDocument()
    expect(screen.queryByText('Members')).not.toBeInTheDocument()
    expect(screen.queryByText('Policies')).not.toBeInTheDocument()

    const editorLinks = screen
      .getAllByText('Open in Editor')
      .map((label) => label.closest('a'))
      .filter((link): link is HTMLAnchorElement => link !== null)
    const workspaceEditorLink = editorLinks.find((link) =>
      link.href.endsWith('/orgs/local/workspaces/3/ide'),
    )
    expect(workspaceEditorLink).toBeDefined()

    await user.click(workspaceEditorLink!)
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/orgs/local/workspaces/3/ide')
      expect(router.state.location.search).toEqual({})
      expect(document.title).toBe('Analytics | Editor | SQLWarden')
    })
  })
})
