import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { instanceSettingsFixture, organizationFixture } from '#/test/fixtures'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { setupStatusHandler } from '#/test/handlers'
import { getRouter } from '#/router'
import { DesktopRuntimeContext } from '#/lib/desktop/context'

describe('desktop settings', () => {
  beforeEach(() => {
    setAccessToken('desktop-token')
    server.use(
      setupStatusHandler({
        configured: true,
        access_mode: 'single_user',
        deployment_mode: 'desktop',
      }),
    )
    window.go = {
      main: {
        DesktopBridge: {
          StartSession: async () => ({
            access_token: 'desktop-token',
            auth_session_id: 'session-1',
            identity: { account_id: 1, org_id: 1, org_slug: 'local', workspace_id: 1 },
          }),
          GetInfo: async () => ({
            version: '0.9.0',
            paths: {
              data_dir: '/desktop/data',
              database: '/desktop/data/sqlwarden.db',
              files: '/desktop/data/files',
              logs: '/desktop/data/logs',
              config_file: '/desktop/data/desktop.json',
            },
          }),
          RevealDataDirectory: async () => undefined,
          RevealLogDirectory: async () => undefined,
        },
      },
    }
  })

  afterEach(() => {
    delete window.go
  })

  it('renders only focused local data controls and storage information', async () => {
    server.use(
      http.get('/api/v1/orgs/local', () =>
        HttpResponse.json(
          organizationFixture({
            slug: 'local',
            schema_snapshots_enabled: true,
            mask_connection_credentials_on_edit: false,
          }),
        ),
      ),
      http.get('/api/v1/instance/settings', () =>
        HttpResponse.json(instanceSettingsFixture({ query_cursor_page_size: 200 })),
      ),
    )

    const { user } = renderDesktopSettings()
    expect(await screen.findByRole('heading', { name: 'Desktop Settings' })).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: 'Cursor page size' })).toHaveValue(200)
    expect(screen.queryByText('SMTP')).not.toBeInTheDocument()
    expect(screen.queryByText('Users')).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'About & Storage' }))
    expect(await screen.findByText('/desktop/data/sqlwarden.db')).toBeInTheDocument()
    expect(screen.getByText('0.9.0')).toBeInTheDocument()
  })

  it('patches only changed organization and instance fields', async () => {
    let orgPatch: Record<string, unknown> | undefined
    let instancePatch: Record<string, unknown> | undefined
    server.use(
      http.get('/api/v1/orgs/local', () =>
        HttpResponse.json(
          organizationFixture({
            slug: 'local',
            schema_snapshots_enabled: true,
            mask_connection_credentials_on_edit: false,
          }),
        ),
      ),
      http.get('/api/v1/instance/settings', () => HttpResponse.json(instanceSettingsFixture())),
      http.patch('/api/v1/orgs/local', async ({ request }) => {
        orgPatch = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(organizationFixture({ slug: 'local' }))
      }),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        instancePatch = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(instanceSettingsFixture())
      }),
    )

    const { user } = renderDesktopSettings()
    const mask = await screen.findByRole('checkbox', {
      name: 'Mask connection credentials on edit',
    })
    await user.click(mask)
    const pageSize = screen.getByRole('spinbutton', { name: 'Cursor page size' })
    await user.clear(pageSize)
    await user.type(pageSize, '250')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(orgPatch).toEqual({ mask_connection_credentials_on_edit: true })
      expect(instancePatch).toEqual({ query_cursor_page_size: 250 })
    })
  })
})

function renderDesktopSettings() {
  const queryClient = createTestQueryClient()
  const router = getRouter({
    history: createMemoryHistory({ initialEntries: ['/desktop/settings'] }),
  })
  const session = {
    access_token: 'desktop-token',
    auth_session_id: 'session-1',
    identity: { account_id: 1, org_id: 1, org_slug: 'local', workspace_id: 1 },
  }
  return {
    user: userEvent.setup(),
    ...render(
      <QueryClientProvider client={queryClient}>
        <DesktopRuntimeContext.Provider value={{ native: true, session }}>
          <RouterProvider router={router} />
        </DesktopRuntimeContext.Provider>
      </QueryClientProvider>,
    ),
  }
}
