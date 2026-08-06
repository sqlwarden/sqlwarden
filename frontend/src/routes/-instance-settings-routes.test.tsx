import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { apiErrorHandler, sessionHandler, setupStatusHandler } from '#/test/handlers'
import {
  instanceConfigurationFixture,
  instanceSettingsFixture,
  sessionFixture,
} from '#/test/fixtures'

function instanceSettingsHandler(settings = instanceSettingsFixture()) {
  return http.get('/api/v1/instance/settings', () => HttpResponse.json(settings))
}

function instanceConfigurationHandler(configuration = instanceConfigurationFixture()) {
  return http.get('/api/v1/instance/configuration', () => HttpResponse.json(configuration))
}

describe('instance settings route', () => {
  beforeEach(() => {
    setAccessToken('test-token')
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [], is_instance_admin: true })),
    )
  })

  it('renders every settings section with values loaded from the API', async () => {
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(instanceConfigurationFixture({ file_storage_mode: 'object' })),
    )

    renderRoute('/administration/instance')

    expect(await screen.findByRole('heading', { name: 'Instance' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'General' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Authentication & Sessions' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Query & Export Limits' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Schema Snapshots' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'File Revisions' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Error Notifications' })).toBeInTheDocument()
    expect(screen.getByDisplayValue('SQLWarden')).toBeInTheDocument()
    // 3,600 seconds round-trips to the largest evenly-dividing unit (hours).
    expect(screen.getByRole('spinbutton', { name: 'Access token lifetime' })).toHaveValue(1)
  })

  it('disables enabling file revisions when the storage mode does not support them', async () => {
    server.use(
      instanceSettingsHandler(instanceSettingsFixture({ file_revisions_enabled: false })),
      instanceConfigurationHandler(instanceConfigurationFixture({ file_storage_mode: 'file' })),
    )

    renderRoute('/administration/instance')

    const checkbox = await screen.findByRole('checkbox', { name: /Enable file revisions/ })
    expect(checkbox).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByText('Not available with the current file storage mode.'),
    ).toBeInTheDocument()
  })

  it('shows a distinct message when runtime settings are unavailable', async () => {
    server.use(
      apiErrorHandler(
        'get',
        '/api/v1/instance/settings',
        503,
        'Runtime settings are temporarily unavailable.',
        'settings_unavailable',
      ),
      instanceConfigurationHandler(),
    )

    renderRoute('/administration/instance')

    expect(
      await screen.findByText('Runtime settings are temporarily unavailable. Try again shortly.'),
    ).toBeInTheDocument()
  })

  it('surfaces field validation errors from the API on save', async () => {
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Validation failed.',
              field_errors: { instance_name: 'Instance name is required.' },
            },
          },
          { status: 422 },
        ),
      ),
    )
    const { user } = renderRoute('/administration/instance')

    const nameInput = await screen.findByDisplayValue('SQLWarden')
    await user.clear(nameInput)
    await user.type(nameInput, 'Updated Name')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(await screen.findByText('Instance name is required.')).toBeInTheDocument()
  })

  it('converts an edited duration amount back to seconds when saving', async () => {
    let capturedBody: unknown
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = await request.json()
        return HttpResponse.json(instanceSettingsFixture({ jwt_access_token_ttl_seconds: 7_200 }))
      }),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByDisplayValue('SQLWarden')
    // jwt_access_token_ttl_seconds fixture value (3,600s) displays as "1" hour.
    const hoursInput = screen.getByRole('spinbutton', { name: 'Access token lifetime' })
    await user.clear(hoursInput)
    await user.type(hoursInput, '2')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() =>
      expect(
        (capturedBody as { jwt_access_token_ttl_seconds: number }).jwt_access_token_ttl_seconds,
      ).toBe(7_200),
    )
  })
})

describe('deployment configuration route', () => {
  beforeEach(() => {
    setAccessToken('test-token')
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [], is_instance_admin: true })),
    )
  })

  it('shows restart-scoped deployment values as read-only data', async () => {
    server.use(
      instanceConfigurationHandler(
        instanceConfigurationFixture({
          base_url: 'https://deployment.example.com',
          http_port: 8443,
          tls_enabled: true,
          restart_required: true,
        }),
      ),
    )

    renderRoute('/administration/configuration')

    expect(await screen.findByRole('heading', { name: 'Configuration' })).toBeInTheDocument()
    expect(screen.getByText('Restart required')).toBeInTheDocument()
    expect(screen.getByText('https://deployment.example.com')).toBeInTheDocument()
    expect(screen.getByText('8443')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save/i })).not.toBeInTheDocument()
  })

  it('does not expose deployment secrets', async () => {
    server.use(instanceConfigurationHandler())

    renderRoute('/administration/configuration')

    await screen.findByRole('heading', { name: 'Configuration' })
    expect(screen.queryByText(/password|secret|token/i)).not.toBeInTheDocument()
  })
})
