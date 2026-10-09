import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Environment } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { connectionFieldsHandler } from '#/test/handlers'
import { emptySshState } from './ConnectionSshFields'
import { drivers } from './connection-drivers'
import { useConnectionForm } from './useConnectionForm'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const environments: Environment[] = [
  {
    id: 4,
    workspace_id: 3,
    name: 'Development',
    created_at: '',
    updated_at: '',
  },
  {
    id: 5,
    workspace_id: 3,
    name: 'Production',
    created_at: '',
    updated_at: '',
  },
]

function stubEngine(overrides: Record<string, unknown> = {}) {
  server.use(
    http.get('/api/v1/engines/:engine', ({ params }) =>
      HttpResponse.json({
        id: params.engine,
        display_name: String(params.engine),
        dialect: String(params.engine),
        capabilities: {},
        supports_system_objects: false,
        show_all_databases: false,
        ...overrides,
      }),
    ),
  )
}

describe('useConnectionForm', () => {
  const queryClient = createTestQueryClient()
  let onOpenChange: ReturnType<typeof vi.fn>

  beforeEach(() => {
    queryClient.clear()
    onOpenChange = vi.fn()
    stubEngine()
    server.use(connectionFieldsHandler())
  })

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  function renderForm(
    options: { lockedEnvironmentId?: number; environments?: Environment[] } = {},
  ) {
    return renderHook(
      () =>
        useConnectionForm({
          open: true,
          onOpenChange,
          orgSlug: 'acme',
          workspaceId: 3,
          environments: options.environments ?? environments,
          lockedEnvironmentId: options.lockedEnvironmentId,
        }),
      { wrapper },
    )
  }

  async function fillRequiredFields(result: ReturnType<typeof renderForm>['result']) {
    await waitFor(() => expect(result.current.fieldSpec.isSuccess).toBe(true))
    act(() => {
      result.current.changeName('Warehouse')
      for (const field of result.current.resolvedFields.filter(
        (candidate) => candidate.required && !candidate.secret,
      )) {
        result.current.changeField(field.key, field.default ?? `${field.key}-value`)
      }
    })
  }

  it('defaults and locks the selected environment while resetting on close', async () => {
    const { result } = renderForm({ lockedEnvironmentId: 5 })
    await waitFor(() => expect(result.current.environmentId).toBe('5'))
    act(() => result.current.pickDriver(drivers[0].id))
    act(() => result.current.changeName('Temporary'))

    act(() => result.current.handleOpenChange(false))

    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(result.current.stage).toBe('driver')
    expect(result.current.name).toBe('')
    expect(result.current.environmentId).toBe('5')
  })

  it('validates connection name, environment, and every required spec field', async () => {
    const { result } = renderForm({ environments: [] })
    act(() => result.current.pickDriver(drivers[0].id))
    await waitFor(() => expect(result.current.fieldSpec.isSuccess).toBe(true))

    act(() => result.current.submit())

    expect(result.current.errors.name).toBe('Name is required.')
    expect(result.current.errors.environmentId).toBe('Environment is required.')
    expect(result.current.errors.fields).toEqual({
      host: 'Host is required.',
      username: 'Username is required.',
    })
  })

  it('resets driver fields and connection-test state when the driver changes', async () => {
    const alternate = drivers.find((driver) => driver.id !== drivers[0].id)
    expect(alternate).toBeDefined()
    const { result } = renderForm()
    act(() => result.current.pickDriver(drivers[0].id))
    act(() => result.current.changeField('host', 'db.internal'))
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/test', () =>
        HttpResponse.json({ ok: true, latency_ms: 9 }),
      ),
    )
    await act(() => result.current.testConnection.mutateAsync())
    expect(result.current.testState).toEqual({ status: 'ok', latencyMs: 9 })

    act(() => result.current.pickDriver(alternate!.id))

    expect(result.current.driverId).toBe(alternate!.id)
    expect(result.current.fields.host).not.toBe('db.internal')
    expect(result.current.testState).toEqual({ status: 'idle' })
    await waitFor(() => expect(result.current.fields.host).toBe(''))
  })

  it('creates a validated connection from structured params and invalidates the list', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 }, { status: 201 })
      }),
    )
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body).toEqual(
      expect.objectContaining({
        name: 'Warehouse',
        driver: drivers[0].id,
        environment_id: 4,
        access_mode: 'open',
        params: { host: 'host-value', port: '5432', username: 'username-value' },
        secrets: {},
      }),
    )
    expect(body).not.toHaveProperty('dsn')
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['org-workspace-connections', 'acme', 3] })
  })

  it('sends the TLS configuration in the create payload', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 8 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)
    act(() =>
      result.current.changeTls({
        mode: 'verify-full',
        serverName: 'db.internal',
        caPem: '-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----',
        clientCertPem: '',
      }),
    )

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.tls_config).toEqual({
      mode: 'verify-full',
      server_name: 'db.internal',
      ca_pem: '-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----',
      client_cert_pem: '',
    })
  })

  it('sends typed secrets in the secrets payload and never in params', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 8 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)
    act(() => result.current.bindSecret('password').dispatch({ type: 'edit', value: 'hunter2' }))

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.secrets).toEqual({ password: 'hunter2' })
    expect(body.params).not.toHaveProperty('password')
  })

  it('includes ssh in the create payload when enabled', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 9 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)
    act(() =>
      result.current.changeSsh({
        ...emptySshState,
        enabled: true,
        host: 'bastion.internal',
        user: 'jump',
        authMethod: 'password',
        insecureSkipHostKey: true,
      }),
    )
    act(() => result.current.bindSecret('ssh_password').dispatch({ type: 'edit', value: 'pw' }))

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.ssh_config).toEqual(
      expect.objectContaining({
        enabled: true,
        host: 'bastion.internal',
        user: 'jump',
        auth_method: 'password',
        port: 22,
        insecure_skip_host_key: true,
      }),
    )
    expect(body.secrets).toEqual({ ssh_password: 'pw' })
  })

  it('omits tls_config for mode disable and drops secrets that do not apply', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 8 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)
    act(() => {
      result.current.bindSecret('ssh_password').dispatch({ type: 'edit', value: 'sp' })
      result.current.bindSecret('tls_client_key').dispatch({ type: 'edit', value: 'tk' })
    })

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body).not.toHaveProperty('tls_config')
    expect(body).not.toHaveProperty('ssh_config')
    expect(body.secrets).toEqual({})
  })

  it('does not send ssh_password when the tunnel uses a private key', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 8 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)
    act(() =>
      result.current.changeSsh({
        ...emptySshState,
        enabled: true,
        host: 'bastion.internal',
        user: 'jump',
        authMethod: 'private_key',
      }),
    )
    act(() => {
      result.current.bindSecret('ssh_password').dispatch({ type: 'edit', value: 'stale' })
      result.current.bindSecret('ssh_private_key').dispatch({ type: 'edit', value: 'KEY' })
    })

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.secrets).toEqual({ ssh_private_key: 'KEY' })
  })

  it('omits ssh_config when the tunnel was never enabled', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 10 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body).not.toHaveProperty('ssh_config')
  })

  it('allows an explicitly unscoped connection after discovery', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/test', () =>
        HttpResponse.json({
          ok: true,
          latency_ms: 7,
          scope_discovery: {
            current: [
              { kind: 'database', name: 'analytics' },
              { kind: 'schema', name: 'public' },
            ],
            scopes: [[{ kind: 'database', name: 'analytics' }]],
          },
        }),
      ),
    )
    const { result } = renderForm()
    act(() => result.current.pickDriver(drivers[0].id))
    await fillRequiredFields(result)
    await act(() => result.current.testConnection.mutateAsync())
    expect(result.current.defaultScope).toHaveLength(2)

    act(() => result.current.selectDatabase(''))

    expect(result.current.defaultScope).toEqual([])
    expect(result.current.fields.database).toBe('')
  })

  it('forces show all databases until a database is chosen and sends the effective value', async () => {
    stubEngine({ show_all_databases: true, supports_system_objects: true })
    let body: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 }, { status: 201 })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.environmentId).toBe('4'))
    act(() => result.current.pickDriver('postgres'))
    await waitFor(() => expect(result.current.showAllDatabasesSupported).toBe(true))
    expect(result.current.systemObjectsSupported).toBe(true)
    await fillRequiredFields(result)
    act(() => result.current.changeField('database', ''))

    expect(result.current.showAllDatabasesForced).toBe(true)
    expect(result.current.showAllDatabases).toBe(true)

    act(() => result.current.changeField('database', 'app'))
    expect(result.current.showAllDatabasesForced).toBe(false)
    expect(result.current.showAllDatabases).toBe(false)

    act(() => result.current.changeShowAllDatabases(true))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.show_all_databases).toBe(true)
    expect(body.default_scope).toEqual([{ kind: 'database', name: 'app' }])
  })

  it('hides navigator options the engine does not report', async () => {
    const { result } = renderForm()
    act(() => result.current.pickDriver('postgres'))
    await waitFor(() => expect(result.current.stage).toBe('form'))

    expect(result.current.showAllDatabasesSupported).toBe(false)
    expect(result.current.systemObjectsSupported).toBe(false)
  })
})
