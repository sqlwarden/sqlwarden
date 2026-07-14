import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { sessionHandler, setupStatusHandler } from '#/test/handlers'
import { organizationFixture, sessionFixture } from '#/test/fixtures'

describe('authenticated shell routes', () => {
  beforeEach(() => setAccessToken('test-token'))

  it('routes the settings landing page to the account page', async () => {
    server.use(setupStatusHandler(), sessionHandler(sessionFixture({ organizations: [] })))
    const { router } = renderRoute('/settings')

    await waitFor(() => expect(router.state.location.pathname).toBe('/settings/account'))
    expect(await screen.findByRole('heading', { name: 'Account' })).toBeInTheDocument()
  })

  it('rejects administration routes for a regular account', async () => {
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [], is_instance_admin: false })),
      http.get('/api/v1/account/orgs', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 50, total: 0 }),
      ),
    )
    const { router } = renderRoute('/administration/users')

    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
    expect(
      await screen.findByRole('heading', { name: 'Choose where to continue' }),
    ).toBeInTheDocument()
  })

  it('routes an instance administrator to the default users page', async () => {
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [], is_instance_admin: true })),
      http.get('/api/v1/instance/accounts', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 10, total: 0 }),
      ),
    )
    const { router } = renderRoute('/administration')

    await waitFor(() => expect(router.state.location.pathname).toBe('/administration/users'))
    expect(await screen.findByRole('heading', { name: 'Users' })).toBeInTheDocument()
  })

  it('redirects a direct workspace URL when effective permissions deny its section', async () => {
    const organization = organizationFixture({ slug: 'acme', name: 'Acme' })
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [organization] })),
      http.get('/api/v1/orgs/acme', () => HttpResponse.json(organization)),
      http.get('/api/v1/orgs/acme/workspaces/3', () =>
        HttpResponse.json({
          id: 3,
          org_id: 1,
          owner_type: 'org',
          owner_id: 1,
          name: 'Analytics',
          description: 'Data workspace',
          environment_count: 2,
          connection_count: 4,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({
          resource_type: 'workspace',
          resource_id: 3,
          permissions: ['ws:read'],
        }),
      ),
    )
    const { router } = renderRoute('/orgs/acme/workspaces/3/connections')

    await waitFor(() => expect(router.state.location.pathname).toBe('/orgs/acme/workspaces/3'))
    expect(await screen.findByRole('heading', { name: 'Analytics' })).toBeInTheDocument()
    expect(screen.queryByText('Connections', { selector: 'p' })).not.toBeInTheDocument()
  })
})
