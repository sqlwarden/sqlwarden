import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { sessionHandler, setupStatusHandler } from '#/test/handlers'
import { organizationFixture, sessionFixture } from '#/test/fixtures'

describe('workspace overview route', () => {
  const organization = organizationFixture({ slug: 'acme', name: 'Acme' })

  beforeEach(() => {
    setAccessToken('test-token')
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [organization] })),
      http.get('/api/v1/orgs/acme', () => HttpResponse.json(organization)),
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({ resource_type: 'workspace', resource_id: 999, permissions: [] }),
      ),
    )
  })

  it('shows a not-found state for an invalid workspace id', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/999', () =>
        HttpResponse.json(
          { error: { code: 'not_found', message: 'Workspace not found.' } },
          { status: 404 },
        ),
      ),
    )

    renderRoute('/orgs/acme/workspaces/999')

    expect(await screen.findByText('Workspace not found')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /back to workspaces/i })).toBeInTheDocument()
  })

  it('navigates back to the workspaces list from the not-found state', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/999', () =>
        HttpResponse.json(
          { error: { code: 'not_found', message: 'Workspace not found.' } },
          { status: 404 },
        ),
      ),
      http.get('/api/v1/orgs/acme/workspaces', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 25, total: 0 }),
      ),
    )

    const { router, user } = renderRoute('/orgs/acme/workspaces/999')

    await user.click(await screen.findByRole('button', { name: /back to workspaces/i }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/orgs/acme/workspaces'))
  })

  it('shows a generic error state for a non-404 workspace load failure', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/999', () =>
        HttpResponse.json(
          { error: { code: 'internal_error', message: 'Something broke.' } },
          { status: 500 },
        ),
      ),
    )

    renderRoute('/orgs/acme/workspaces/999')

    expect(await screen.findByText("Couldn't load this workspace")).toBeInTheDocument()
    expect(screen.getByText('Something broke.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /back to workspaces/i })).toBeInTheDocument()
  })

  it('shows stat tiles and a nav list, filtered by permission', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/1', () =>
        HttpResponse.json({
          id: 1,
          owner_type: 'org',
          owner_id: 1,
          name: 'Analytics',
          description: 'Reporting workspace.',
          environment_count: 3,
          connection_count: 5,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        }),
      ),
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({
          resource_type: 'workspace',
          resource_id: 1,
          permissions: ['env:read', 'conn:read', 'ws:read'],
        }),
      ),
    )

    renderRoute('/orgs/acme/workspaces/1')

    expect(await screen.findByRole('heading', { name: 'Analytics' })).toBeInTheDocument()

    expect(screen.getByRole('link', { name: /environments\s*3/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /connections\s*5/i })).toBeInTheDocument()
    expect(screen.getByText('Workspace name and configuration.')).toBeInTheDocument()
    expect(screen.queryByText('People and teams with workspace access.')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Roles and access policies in this workspace.'),
    ).not.toBeInTheDocument()
  })
})
