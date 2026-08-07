import { screen, waitFor, within } from '@testing-library/react'
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

    const { user } = renderRoute('/administration/instance')

    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
    expect(screen.queryByText('Runtime editable')).not.toBeInTheDocument()
    expect(screen.queryByText('Deployment read-only')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'General' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Authentication & Sessions' })).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Data' }))
    expect(screen.getByRole('heading', { name: 'Query & Export Limits' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Schema Snapshots' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'File Revisions' })).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: 'Query cursor page size' })).toHaveValue(200)

    await user.click(screen.getByRole('tab', { name: 'Operations' }))
    expect(screen.getByRole('heading', { name: 'Error Notifications' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Logging' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Background Jobs' })).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Email' }))
    expect(screen.getByRole('heading', { name: 'Email Delivery' })).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'General' }))
    expect(screen.getByDisplayValue('SQLWarden')).toBeInTheDocument()
    // 3,600 seconds round-trips to the largest evenly-dividing unit (hours).
    expect(screen.getByRole('spinbutton', { name: 'Access token lifetime' })).toHaveValue(1)
  })

  it('keeps save actions beside the tabs and reports unsaved changes', async () => {
    server.use(instanceSettingsHandler(), instanceConfigurationHandler())
    const { user } = renderRoute('/administration/instance')

    const actions = await screen.findByRole('region', { name: 'Settings actions' })
    const saveButton = screen.getByRole('button', { name: 'Save settings' })
    const tabList = screen.getByRole('tablist')

    expect(actions).not.toHaveClass('sticky')
    expect(actions.parentElement).toContainElement(tabList)
    expect(tabList.parentElement).toHaveClass('overflow-y-hidden')
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(saveButton).toBeDisabled()

    const nameInput = screen.getByRole('textbox', { name: 'Instance name' })
    await user.clear(nameInput)
    await user.type(nameInput, 'Updated SQLWarden')

    expect(screen.getByRole('status')).toHaveTextContent('You have unsaved changes.')
    expect(saveButton).toBeEnabled()
  })

  it('saves the instance base URL as the canonical runtime URL', async () => {
    let capturedBody: Record<string, unknown> | undefined
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(
          instanceSettingsFixture({ base_url: 'https://updated.example.com' }),
        )
      }),
    )
    const { user } = renderRoute('/administration/instance')

    const baseURL = await screen.findByRole('textbox', { name: 'Base URL' })
    await user.clear(baseURL)
    await user.type(baseURL, 'https://updated.example.com')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(capturedBody?.base_url).toBe('https://updated.example.com'))
    expect(capturedBody).not.toHaveProperty('public_url')
  })

  it('disables enabling file revisions when the storage mode does not support them', async () => {
    server.use(
      instanceSettingsHandler(instanceSettingsFixture({ file_revisions_enabled: false })),
      instanceConfigurationHandler(instanceConfigurationFixture({ file_storage_mode: 'file' })),
    )

    const { user } = renderRoute('/administration/instance')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Data' }))
    const checkbox = screen.getByRole('checkbox', { name: /Enable file revisions/ })
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

  it('opens the section containing the first server-side validation error', async () => {
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Validation failed.',
              field_errors: { smtp_host: 'SMTP host is required.' },
            },
          },
          { status: 422 },
        ),
      ),
    )
    const { user } = renderRoute('/administration/instance')

    const nameInput = await screen.findByRole('textbox', { name: 'Instance name' })
    await user.clear(nameInput)
    await user.type(nameInput, 'Updated SQLWarden')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(await screen.findByText('SMTP host is required.')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Email' })).toHaveAttribute('aria-selected', 'true')
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

  it('sends an edited query cursor page size when saving', async () => {
    let capturedBody: Record<string, unknown> | undefined
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(instanceSettingsFixture({ query_cursor_page_size: 500 }))
      }),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Data' }))
    const pageSizeInput = screen.getByRole('spinbutton', {
      name: 'Query cursor page size',
    })
    await user.clear(pageSizeInput)
    await user.type(pageSizeInput, '500')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(capturedBody?.query_cursor_page_size).toBe(500))
  })

  it('surfaces field validation errors for the query cursor page size', async () => {
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Validation failed.',
              field_errors: {
                query_cursor_page_size: 'Query cursor page size must be greater than 0.',
              },
            },
          },
          { status: 422 },
        ),
      ),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Data' }))
    const pageSizeInput = screen.getByRole('spinbutton', {
      name: 'Query cursor page size',
    })
    await user.clear(pageSizeInput)
    await user.type(pageSizeInput, '500')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(
      await screen.findByText('Query cursor page size must be greater than 0.'),
    ).toBeInTheDocument()
  })

  it('renders the logging, background jobs, and email delivery sections with values loaded from the API', async () => {
    server.use(
      instanceSettingsHandler(
        instanceSettingsFixture({
          log_level: 'warn',
          database_query_tracing_enabled: true,
          access_logs_enabled: true,
          jobs_worker_count: 24,
          jobs_poll_interval_seconds: 5,
          jobs_claim_lease_seconds: 600,
          jobs_completed_retention_seconds: 1_209_600,
          smtp_enabled: true,
          smtp_host: 'smtp.example.com',
          smtp_port: 587,
          smtp_username: 'mailer',
          smtp_password_configured: true,
          smtp_from: 'alerts@example.com',
        }),
      ),
      instanceConfigurationHandler(),
    )

    const { user } = renderRoute('/administration/instance')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Operations' }))
    expect(screen.getByRole('heading', { name: 'Logging' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Background Jobs' })).toBeInTheDocument()

    expect(screen.getByRole('combobox', { name: 'Log level' })).toHaveTextContent('Warn')
    expect(screen.getByRole('checkbox', { name: 'Enable database query tracing' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Enable HTTP access logs' })).toBeChecked()

    expect(screen.getByRole('spinbutton', { name: 'Worker count' })).toHaveValue(24)
    // 5s doesn't divide evenly into a larger unit, so it round-trips as seconds.
    expect(screen.getByRole('spinbutton', { name: 'Poll interval' })).toHaveValue(5)
    // 600s round-trips to the largest evenly-dividing unit (10 minutes).
    expect(screen.getByRole('spinbutton', { name: 'Claim lease' })).toHaveValue(10)
    // 1,209,600s round-trips to the largest evenly-dividing unit (14 days).
    expect(screen.getByRole('spinbutton', { name: 'Completed job retention' })).toHaveValue(14)

    await user.click(screen.getByRole('tab', { name: 'Email' }))
    expect(screen.getByRole('heading', { name: 'Email Delivery' })).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Enable SMTP' })).toBeChecked()
    expect(screen.getByDisplayValue('smtp.example.com')).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: 'SMTP port' })).toHaveValue(587)
    expect(screen.getByDisplayValue('mailer')).toBeInTheDocument()
    expect(screen.getByDisplayValue('alerts@example.com')).toBeInTheDocument()
    expect(screen.getByText('Password configured.')).toBeInTheDocument()
    expect(screen.getByLabelText('SMTP password')).toHaveValue('')
  })

  it('sends the correct numeric and boolean payload for operational settings', async () => {
    let capturedBody: Record<string, unknown> | undefined
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(
          instanceSettingsFixture({
            log_level: 'debug',
            database_query_tracing_enabled: true,
            access_logs_enabled: true,
            jobs_worker_count: 32,
          }),
        )
      }),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByDisplayValue('SQLWarden')
    await user.click(screen.getByRole('tab', { name: 'Operations' }))

    await user.click(screen.getByRole('combobox', { name: 'Log level' }))
    await user.click(screen.getByRole('option', { name: 'Debug' }))

    await user.click(screen.getByRole('checkbox', { name: 'Enable database query tracing' }))
    await user.click(screen.getByRole('checkbox', { name: 'Enable HTTP access logs' }))

    const workerCount = screen.getByRole('spinbutton', { name: 'Worker count' })
    await user.clear(workerCount)
    await user.type(workerCount, '32')

    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(capturedBody).toBeDefined())
    expect(capturedBody).toMatchObject({
      log_level: 'debug',
      database_query_tracing_enabled: true,
      access_logs_enabled: true,
      jobs_worker_count: 32,
    })
    expect(capturedBody?.smtp_password_configured).toBeUndefined()
  })

  it('omits the saved SMTP password when the password field is left blank', async () => {
    let capturedBody: Record<string, unknown> | undefined
    server.use(
      instanceSettingsHandler(
        instanceSettingsFixture({
          smtp_enabled: true,
          smtp_username: 'mailer',
          smtp_password_configured: true,
        }),
      ),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(instanceSettingsFixture({ smtp_username: 'mailer-updated' }))
      }),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Email' }))
    const usernameInput = screen.getByDisplayValue('mailer')
    await user.clear(usernameInput)
    await user.type(usernameInput, 'mailer-updated')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(capturedBody).toBeDefined())
    expect(capturedBody).not.toHaveProperty('smtp_password')
    expect(capturedBody).not.toHaveProperty('smtp_password_configured')
  })

  it('sends a new SMTP password when one is entered', async () => {
    let capturedBody: Record<string, unknown> | undefined
    server.use(
      instanceSettingsHandler(
        instanceSettingsFixture({ smtp_enabled: true, smtp_password_configured: false }),
      ),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(instanceSettingsFixture({ smtp_password_configured: true }))
      }),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByDisplayValue('SQLWarden')
    await user.click(screen.getByRole('tab', { name: 'Email' }))
    expect(screen.getByText('No password configured.')).toBeInTheDocument()

    await user.type(screen.getByLabelText('SMTP password'), 's3cret')
    expect(screen.getByText('A new password will be saved.')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(capturedBody).toBeDefined())
    expect(capturedBody?.smtp_password).toBe('s3cret')

    // Local password state resets from the response after a successful save.
    expect(await screen.findByText('Password configured.')).toBeInTheDocument()
    expect(screen.getByLabelText('SMTP password')).toHaveValue('')
  })

  it('sends smtp_password: null when the saved password is explicitly cleared', async () => {
    let capturedBody: Record<string, unknown> | undefined
    server.use(
      instanceSettingsHandler(
        instanceSettingsFixture({ smtp_enabled: true, smtp_password_configured: true }),
      ),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(instanceSettingsFixture({ smtp_password_configured: false }))
      }),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByDisplayValue('SQLWarden')
    await user.click(screen.getByRole('tab', { name: 'Email' }))
    await user.click(screen.getByRole('button', { name: 'Clear saved password' }))
    expect(screen.getByText('Password will be cleared when you save.')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(capturedBody).toBeDefined())
    expect(capturedBody).toHaveProperty('smtp_password', null)
    expect(capturedBody).not.toHaveProperty('smtp_password_configured')
  })

  it('surfaces field validation errors for the new operational settings', async () => {
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(),
      http.patch('/api/v1/instance/settings', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Validation failed.',
              field_errors: {
                jobs_worker_count: 'Worker count must be between 1 and 256.',
                smtp_host: 'SMTP host must be 253 characters or fewer.',
              },
            },
          },
          { status: 422 },
        ),
      ),
    )
    const { user } = renderRoute('/administration/instance')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Operations' }))
    const workerCount = screen.getByRole('spinbutton', { name: 'Worker count' })
    await user.clear(workerCount)
    await user.type(workerCount, '32')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(await screen.findByText('Worker count must be between 1 and 256.')).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Email' }))
    expect(screen.getByText('SMTP host must be 253 characters or fewer.')).toBeInTheDocument()
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

  it('redirects the legacy configuration route to read-only deployment data', async () => {
    server.use(
      instanceSettingsHandler(),
      instanceConfigurationHandler(
        instanceConfigurationFixture({
          http_port: 8443,
          tls_enabled: true,
          restart_required: true,
        }),
      ),
    )

    const { user } = renderRoute('/administration/configuration')

    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Deployment' }))
    expect(screen.getByText('Restart required')).toBeInTheDocument()
    expect(screen.getByText('8443')).toBeInTheDocument()
    expect(screen.getByText(/resolved at process startup/)).toBeInTheDocument()
    const deploymentPanel = screen
      .getByText('Deployment-managed configuration')
      .closest('[role="tabpanel"]') as HTMLElement
    expect(within(deploymentPanel).queryByRole('textbox')).not.toBeInTheDocument()
    expect(within(deploymentPanel).queryByRole('button', { name: /save/i })).not.toBeInTheDocument()
  })

  it('does not expose deployment secrets', async () => {
    server.use(instanceSettingsHandler(), instanceConfigurationHandler())

    const { user } = renderRoute('/administration/configuration')

    await screen.findByRole('heading', { name: 'Settings' })
    await user.click(screen.getByRole('tab', { name: 'Deployment' }))
    const deploymentPanel = screen
      .getByText('Deployment-managed configuration')
      .closest('[role="tabpanel"]') as HTMLElement
    expect(within(deploymentPanel).queryByText(/password|secret|token/i)).not.toBeInTheDocument()
  })
})
