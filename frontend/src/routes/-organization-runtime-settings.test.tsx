import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import type { OrganizationRuntimeSettingsPatch } from '#/lib/api/types'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { apiErrorHandler, sessionHandler, setupStatusHandler } from '#/test/handlers'
import {
  organizationFixture,
  organizationRuntimeSettingsFixture,
  sessionFixture,
} from '#/test/fixtures'

const organization = organizationFixture({ slug: 'acme', name: 'Acme' })

function runtimeSettingsHandler(settings = organizationRuntimeSettingsFixture()) {
  return http.get('/api/v1/orgs/acme/runtime-settings', () => HttpResponse.json(settings))
}

function permissionsHandler(permissions: string[]) {
  return http.get('/api/v1/orgs/acme/permissions/effective', () =>
    HttpResponse.json({ resource_type: 'org', resource_id: 1, permissions }),
  )
}

describe('organization runtime policy route', () => {
  beforeEach(() => {
    setAccessToken('test-token')
    server.use(
      setupStatusHandler(),
      sessionHandler(sessionFixture({ organizations: [organization] })),
      http.get('/api/v1/orgs/acme', () => HttpResponse.json(organization)),
    )
  })

  it('shows inherited effective values and instance constraints', async () => {
    server.use(runtimeSettingsHandler(), permissionsHandler(['org:read', 'org:write']))

    renderRoute('/orgs/acme/settings/runtime')

    await screen.findAllByText(/Inherits instance default:/)
    expect(screen.getByRole('heading', { name: 'Runtime Policy' })).toBeInTheDocument()
    expect(screen.getAllByText(/Inherits instance default:/)).toHaveLength(7)
    expect(screen.getByText('Instance limit: 1,000 rows')).toBeInTheDocument()
    expect(screen.getByText('Instance minimum interval: 1 hour')).toBeInTheDocument()
  })

  it('creates an override and sends only the changed field', async () => {
    let capturedBody: OrganizationRuntimeSettingsPatch | undefined
    server.use(
      runtimeSettingsHandler(),
      permissionsHandler(['org:read', 'org:write']),
      http.patch('/api/v1/orgs/acme/runtime-settings', async ({ request }) => {
        capturedBody = (await request.json()) as OrganizationRuntimeSettingsPatch
        return HttpResponse.json(
          organizationRuntimeSettingsFixture({
            overrides: {
              ...organizationRuntimeSettingsFixture().overrides,
              query_max_result_rows: 500,
            },
            effective: {
              ...organizationRuntimeSettingsFixture().effective,
              query_max_result_rows: 500,
            },
          }),
        )
      }),
    )
    const { user } = renderRoute('/orgs/acme/settings/runtime')

    const override = await screen.findByRole('checkbox', { name: 'Override Query row limit' })
    await user.click(override)
    const input = screen.getByRole('spinbutton', { name: 'Query row limit' })
    expect(input).toHaveAttribute('max', '1000')
    await user.clear(input)
    await user.type(input, '500')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(capturedBody).toEqual({ query_max_result_rows: 500 }))
  })

  it('uses null to return an existing override to inheritance', async () => {
    let capturedBody: OrganizationRuntimeSettingsPatch | undefined
    const base = organizationRuntimeSettingsFixture()
    const current = organizationRuntimeSettingsFixture({
      overrides: { ...base.overrides, query_max_result_rows: 500 },
      effective: { ...base.effective, query_max_result_rows: 500 },
    })
    server.use(
      runtimeSettingsHandler(current),
      permissionsHandler(['org:read', 'org:write']),
      http.patch('/api/v1/orgs/acme/runtime-settings', async ({ request }) => {
        capturedBody = (await request.json()) as OrganizationRuntimeSettingsPatch
        return HttpResponse.json(organizationRuntimeSettingsFixture())
      }),
    )
    const { user } = renderRoute('/orgs/acme/settings/runtime')

    const override = await screen.findByRole('checkbox', { name: 'Override Query row limit' })
    expect(override).toBeChecked()
    await user.click(override)
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(capturedBody).toEqual({ query_max_result_rows: null }))
  })

  it('keeps inherit controls available when file revisions become unavailable', async () => {
    let capturedBody: OrganizationRuntimeSettingsPatch | undefined
    const base = organizationRuntimeSettingsFixture()
    server.use(
      runtimeSettingsHandler(
        organizationRuntimeSettingsFixture({
          overrides: { ...base.overrides, file_revisions_enabled: true },
          effective: { ...base.effective, file_revisions_enabled: false },
          constraints: { ...base.constraints, file_revisions_available: false },
        }),
      ),
      permissionsHandler(['org:read', 'org:write']),
      http.patch('/api/v1/orgs/acme/runtime-settings', async ({ request }) => {
        capturedBody = (await request.json()) as OrganizationRuntimeSettingsPatch
        return HttpResponse.json(organizationRuntimeSettingsFixture())
      }),
    )
    const { user } = renderRoute('/orgs/acme/settings/runtime')

    const override = await screen.findByRole('checkbox', {
      name: 'Override Enable file revisions',
    })
    expect(override).not.toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('checkbox', { name: 'File revisions enabled' })).toHaveAttribute(
      'aria-disabled',
      'true',
    )
    await user.click(override)
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(capturedBody).toEqual({ file_revisions_enabled: null }))
  })

  it('only allows an unlimited background export override when the instance is unlimited', async () => {
    const base = organizationRuntimeSettingsFixture()
    server.use(
      runtimeSettingsHandler(
        organizationRuntimeSettingsFixture({
          effective: { ...base.effective, exports_background_max_bytes: 10_485_760 },
          constraints: { ...base.constraints, exports_background_max_bytes_max: 10_485_760 },
        }),
      ),
      permissionsHandler(['org:read', 'org:write']),
    )
    const { user } = renderRoute('/orgs/acme/settings/runtime')

    await user.click(
      await screen.findByRole('checkbox', { name: 'Override Background export limit' }),
    )

    const input = screen.getByRole('spinbutton', { name: 'Background export limit' })
    expect(Number(input.getAttribute('min'))).toBeGreaterThan(0)
    expect(input).toHaveAttribute('max', '10')
    expect(screen.queryByText(/0 means unlimited/)).not.toBeInTheDocument()
  })

  it('renders read-only policy for an organization reader', async () => {
    server.use(runtimeSettingsHandler(), permissionsHandler(['org:read']))

    renderRoute('/orgs/acme/settings/runtime')

    const override = await screen.findByRole('checkbox', { name: 'Override Query row limit' })
    expect(override).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByText(/organization write permission/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  it('surfaces field validation errors returned by the API', async () => {
    server.use(
      runtimeSettingsHandler(),
      permissionsHandler(['org:read', 'org:write']),
      http.patch('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Validation failed.',
              field_errors: { query_max_result_rows: 'Must not exceed the instance limit.' },
            },
          },
          { status: 422 },
        ),
      ),
    )
    const { user } = renderRoute('/orgs/acme/settings/runtime')

    await user.click(await screen.findByRole('checkbox', { name: 'Override Query row limit' }))
    const input = screen.getByRole('spinbutton', { name: 'Query row limit' })
    await user.clear(input)
    await user.type(input, '500')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(await screen.findByText('Must not exceed the instance limit.')).toBeInTheDocument()
    expect(input).toHaveAttribute('aria-invalid', 'true')
  })

  it('shows a distinct message when runtime settings are unavailable', async () => {
    server.use(
      apiErrorHandler(
        'get',
        '/api/v1/orgs/acme/runtime-settings',
        503,
        'Runtime settings are temporarily unavailable.',
        'settings_unavailable',
      ),
      permissionsHandler(['org:read', 'org:write']),
    )

    renderRoute('/orgs/acme/settings/runtime')

    expect(
      await screen.findByText('Runtime settings are temporarily unavailable. Try again shortly.'),
    ).toBeInTheDocument()
  })
})
