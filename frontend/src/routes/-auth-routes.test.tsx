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
    expect(document.title).toBe('Login | SQLWarden')
  })

  it('redirects an unconfigured instance from login to setup', async () => {
    server.use(setupStatusHandler(setupStatusFixture({ configured: false })))

    renderRoute('/login')

    expect(
      await screen.findByRole('heading', { name: 'Create the instance admin' }),
    ).toBeInTheDocument()
    expect(document.title).toBe('Setup | SQLWarden')
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

  it('returns to the complete internal URL after login', async () => {
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
      http.get('/api/v1/account', () =>
        HttpResponse.json({
          id: 1,
          email: 'alex@example.com',
          name: 'Alex Ward',
          is_active: true,
          created_at: '',
          updated_at: '',
        }),
      ),
    )
    const redirect = '/settings/account?section=password#password'
    const { router, user } = renderRoute(`/login?redirect=${encodeURIComponent(redirect)}`)

    await user.type(await screen.findByRole('textbox'), 'alex@example.com')
    await user.type(document.querySelector<HTMLInputElement>('input[type="password"]')!, 'secret')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => expect(router.state.location.href).toBe(redirect))
    expect(localStorage.getItem('sqlwarden.access_token')).toBe('new-token')
  })

  it('redirects anonymous landing-page visits to login', async () => {
    server.use(setupStatusHandler())
    const { router } = renderRoute('/')

    await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
    expect(router.state.location.search).toEqual({ redirect: '/' })
  })

  it('preserves an anonymous deep link, including search and hash, for login', async () => {
    server.use(setupStatusHandler())
    const { router } = renderRoute('/settings/api-tokens?status=active#token-list')

    await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
    expect(router.state.location.search).toEqual({
      redirect: '/settings/api-tokens?status=active#token-list',
    })
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

  it('asks an existing invited account to sign in and preserves the invitation return path', async () => {
    server.use(
      http.get('/api/v1/invitations/existing-token', () =>
        HttpResponse.json({
          organization: { id: 4, slug: 'acme', name: 'Acme', created_at: '', updated_at: '' },
          email: 'invitee@example.com',
          expires_at: '2026-08-11T00:00:00Z',
          status: 'pending',
          account_exists: true,
          authenticated_as_invitee: false,
        }),
      ),
    )

    renderRoute('/invitations/existing-token')

    expect(await screen.findByRole('heading', { name: 'Join Acme' })).toBeInTheDocument()
    expect(document.title).toBe('Acme invitation | SQLWarden')
    expect(screen.getByRole('button', { name: 'Sign in to accept' })).toHaveAttribute(
      'href',
      '/login?redirect=%2Finvitations%2Fexisting-token',
    )
  })

  it('shows account creation fields for a new invited email', async () => {
    server.use(
      http.get('/api/v1/invitations/new-token', () =>
        HttpResponse.json({
          organization: { id: 4, slug: 'acme', name: 'Acme', created_at: '', updated_at: '' },
          email: 'new@example.com',
          expires_at: '2026-08-11T00:00:00Z',
          status: 'pending',
          account_exists: false,
          authenticated_as_invitee: false,
        }),
      ),
    )

    const { user } = renderRoute('/invitations/new-token')

    expect(await screen.findByRole('heading', { name: 'Join Acme' })).toBeInTheDocument()
    const submit = screen.getByRole('button', { name: 'Create account and join' })
    expect(submit).toBeDisabled()
    await user.type(screen.getByLabelText('Name'), 'New Invitee')
    await user.type(screen.getByLabelText('Password'), 'securepass99')
    expect(submit).toBeEnabled()
  })
})
