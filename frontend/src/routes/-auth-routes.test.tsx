import { screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse } from 'msw'
import { setAccessToken } from '#/lib/auth/access-token'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { setupStatusHandler } from '#/test/handlers'
import { setupStatusFixture } from '#/test/fixtures'

describe('authentication route behavior', () => {
  it('renders the login form for a configured instance', async () => {
    server.use(setupStatusHandler())

    renderRoute('/login')

    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toHaveAttribute('type', 'email')
    expect(document.querySelector('input[type="password"]')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Source code' })).toHaveAttribute(
      'href',
      'https://github.com/sqlwarden/sqlwarden',
    )
  })

  it('redirects an unconfigured instance from login to setup', async () => {
    server.use(setupStatusHandler(setupStatusFixture({ configured: false })))

    renderRoute('/login')

    expect(
      await screen.findByRole('heading', { name: 'Create the instance admin' }),
    ).toBeInTheDocument()
  })

  it('validates setup and derives an organization slug', async () => {
    server.use(setupStatusHandler(setupStatusFixture({ configured: false })))
    const { user } = renderRoute('/setup')

    await user.click(await screen.findByRole('button', { name: 'Create admin and organization' }))
    expect(screen.getByText('Name is required.')).toBeInTheDocument()
    expect(screen.getByText('Organization name is required.')).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('Acme Cloud'), 'Example Platform')
    expect(screen.getByPlaceholderText('acme-cloud')).toHaveValue('example-platform')
  })

  it('submits credentials and redirects after login', async () => {
    server.use(
      setupStatusHandler(),
      http.post('/api/v1/auth/login', () => HttpResponse.json({ access_token: 'new-token' })),
      http.get('/api/v1/session', () =>
        HttpResponse.json({
          account: { id: 1, email: 'alex@example.com', name: 'Alex Ward', is_active: true },
          organizations: [],
          is_instance_admin: false,
          personal_spaces_enabled: false,
        }),
      ),
      http.get('/api/v1/account/orgs', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 50, total: 0 }),
      ),
    )
    const { router, user } = renderRoute('/login')

    await user.type(await screen.findByRole('textbox'), 'alex@example.com')
    const password = document.querySelector<HTMLInputElement>('input[type="password"]')
    expect(password).not.toBeNull()
    await user.type(password!, 'password123')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
    expect(localStorage.getItem('sqlwarden.access_token')).toBe('new-token')
  })

  it('redirects anonymous landing-page visits to login', async () => {
    server.use(setupStatusHandler())
    const { router } = renderRoute('/')

    await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  })

  it('redirects authenticated users away from login', async () => {
    setAccessToken('existing-token')
    server.use(
      setupStatusHandler(),
      http.get('/api/v1/session', () =>
        HttpResponse.json({
          account: { id: 1, email: 'alex@example.com', name: 'Alex Ward', is_active: true },
          organizations: [],
          is_instance_admin: false,
          personal_spaces_enabled: false,
        }),
      ),
      http.get('/api/v1/account/orgs', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 50, total: 0 }),
      ),
    )
    const { router } = renderRoute('/login')

    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  })
})
