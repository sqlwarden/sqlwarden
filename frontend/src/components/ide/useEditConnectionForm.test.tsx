import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Connection, ConnectionDetail } from '#/lib/api/types'
import { connectionDetailFixture } from '#/test/fixtures'
import { connectionFieldsHandler } from '#/test/handlers'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { useEditConnectionForm } from './useEditConnectionForm'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const connection: Connection = {
  id: 7,
  workspace_id: 3,
  environment_id: 2,
  name: 'analytics-pg',
  driver: 'postgres',
  access_mode: 'open',
  show_system_schemas: false,
  show_all_databases: true,
  created_at: '',
  updated_at: '',
}

const connectionPath = '/api/v1/orgs/acme/workspaces/3/connections/7'

function detailHandler(detail: ConnectionDetail = connectionDetailFixture()) {
  return http.get(connectionPath, () => HttpResponse.json(detail))
}

function capturePatch() {
  const captured: { body: Record<string, unknown> } = { body: {} }
  server.use(
    http.patch(connectionPath, async ({ request }) => {
      captured.body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ id: 7 })
    }),
  )
  return captured
}

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

describe('useEditConnectionForm', () => {
  const queryClient = createTestQueryClient()
  let onOpenChange: ReturnType<typeof vi.fn>

  beforeEach(() => {
    queryClient.clear()
    onOpenChange = vi.fn()
    server.use(connectionFieldsHandler(), detailHandler())
    stubEngine()
  })

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  function renderForm(initialOpen = true) {
    return renderHook(
      ({ open }: { open: boolean }) =>
        useEditConnectionForm({
          open,
          onOpenChange,
          orgSlug: 'acme',
          workspaceId: 3,
          connection,
        }),
      { wrapper, initialProps: { open: initialOpen } },
    )
  }

  it('hydrates params and secret states from the connection detail', async () => {
    const { result } = renderForm()

    await waitFor(() => expect(result.current.fields.host).toBe('db.example.test'))
    expect(result.current.fields).toEqual(
      expect.objectContaining({
        port: '5432',
        database: 'analytics',
        username: 'reader',
      }),
    )
    expect(result.current.fields).not.toHaveProperty('password')
    expect(result.current.bindSecret('password').state).toEqual({
      kind: 'saved',
      revealable: true,
    })
    expect(result.current.loading).toBe(false)
  })

  it('hydrates TLS state and sends it in the update payload', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          tls_config: {
            mode: 'verify-ca',
            server_name: 'db.internal',
            ca_pem: '-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----',
            client_cert_pem: '',
          },
        }),
      ),
    )
    const patch = capturePatch()
    const { result } = renderForm()

    await waitFor(() => expect(result.current.tls.mode).toBe('verify-ca'))
    expect(result.current.tls.serverName).toBe('db.internal')

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.tls_config).toEqual({
      mode: 'verify-ca',
      server_name: 'db.internal',
      ca_pem: '-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----',
      client_cert_pem: '',
    })
    expect(patch.body.secrets).toEqual({})
  })

  it('sends null for a saved secret that was cleared and omits untouched secrets', async () => {
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.bindSecret('password').state.kind).toBe('saved'))

    act(() => result.current.bindSecret('password').dispatch({ type: 'clear' }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.secrets).toEqual({ password: null })
    expect(patch.body.params).not.toHaveProperty('password')
  })

  it('omits ssh_config and tls_config when the connection has none stored', async () => {
    let testBody: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/test', async ({ request }) => {
        testBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ ok: true, latency_ms: 1 })
      }),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.requiredFieldsFilled).toBe(true))

    await act(() => result.current.testConnection.mutateAsync())
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    for (const body of [testBody, patch.body]) {
      expect(body).not.toHaveProperty('ssh_config')
      expect(body).not.toHaveProperty('tls_config')
      expect(body.secrets).toEqual({})
    }
  })

  it('includes ssh_config and tls_config when the connection has them stored', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          tls_config: { mode: 'disable', server_name: '', ca_pem: '', client_cert_pem: '' },
          ssh_config: {
            enabled: false,
            host: '',
            port: 22,
            user: '',
            auth_method: 'password',
            known_hosts_entry: '',
            fingerprint: '',
            insecure_skip_host_key: false,
          },
        }),
      ),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.requiredFieldsFilled).toBe(true))

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.tls_config).toMatchObject({ mode: 'disable' })
    expect(patch.body.ssh_config).toMatchObject({ enabled: false })
  })

  it('includes ssh_config when the user enables a tunnel on a connection without one', async () => {
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.requiredFieldsFilled).toBe(true))

    act(() =>
      result.current.changeSsh({
        ...result.current.ssh,
        enabled: true,
        host: 'bastion',
        user: 'j',
      }),
    )
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.ssh_config).toMatchObject({ enabled: true, host: 'bastion' })
  })

  it('does not send ssh or tls secrets that no longer apply', async () => {
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.requiredFieldsFilled).toBe(true))

    act(() => {
      result.current.bindSecret('ssh_password').dispatch({ type: 'edit', value: 'sp' })
      result.current.bindSecret('tls_client_key').dispatch({ type: 'edit', value: 'tk' })
    })
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.secrets).toEqual({})
  })

  it('sends a replacement string for an edited secret', async () => {
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.bindSecret('password').state.kind).toBe('saved'))

    act(() => result.current.bindSecret('password').dispatch({ type: 'edit', value: 'rotated' }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.secrets).toEqual({ password: 'rotated' })
  })

  it('reveals a saved secret through the reveal endpoint without caching the value', async () => {
    let revealed = 0
    server.use(
      http.post(`${connectionPath}/secrets/password/reveal`, () => {
        revealed += 1
        return HttpResponse.json({ value: 'hunter2' })
      }),
    )
    const { result } = renderForm()
    await waitFor(() => expect(result.current.bindSecret('password').state.kind).toBe('saved'))

    let value = ''
    await act(async () => {
      value = await result.current.bindSecret('password').reveal!()
    })

    expect(value).toBe('hunter2')
    expect(revealed).toBe(1)
    const cached = queryClient
      .getMutationCache()
      .getAll()
      .filter((mutation) => mutation.state.data !== undefined)
    expect(cached).toHaveLength(0)
  })

  it('does not offer a reveal action without a connection', () => {
    const { result } = renderHook(
      () =>
        useEditConnectionForm({
          open: false,
          onOpenChange,
          orgSlug: 'acme',
          workspaceId: 3,
          connection: undefined,
        }),
      { wrapper },
    )

    expect(result.current.bindSecret('password').reveal).toBeUndefined()
  })

  it('re-populates the form when the same connection is edited again after closing', async () => {
    const { result, rerender } = renderForm()
    await waitFor(() => expect(result.current.fields.host).toBe('db.example.test'))

    act(() => result.current.handleOpenChange(false))
    rerender({ open: false })
    expect(result.current.fields.host).toBe('')

    rerender({ open: true })
    await waitFor(() => expect(result.current.fields.host).toBe('db.example.test'))
  })

  it('reports a load failure when the detail request fails', async () => {
    server.use(
      http.get(connectionPath, () =>
        HttpResponse.json({ error: { code: 'forbidden', message: 'No.' } }, { status: 403 }),
      ),
    )
    const { result } = renderForm()

    await waitFor(() => expect(result.current.loadFailed).toBe(true))
    expect(result.current.requiredFieldsFilled).toBe(false)
  })

  it('discovers scopes on test, passes connection_id, and includes the selected default_scope', async () => {
    let testBody: Record<string, unknown> = {}
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/test', async ({ request }) => {
        testBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({
          ok: true,
          latency_ms: 5,
          scope_discovery: {
            current: [{ kind: 'database', name: 'analytics' }],
            scopes: [
              [{ kind: 'database', name: 'analytics' }],
              [{ kind: 'database', name: 'reporting' }],
            ],
          },
        })
      }),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.requiredFieldsFilled).toBe(true))

    await act(() => result.current.testConnection.mutateAsync())
    expect(testBody.connection_id).toBe(7)
    expect(testBody.driver).toBe('postgres')
    expect(testBody.params).toEqual(
      expect.objectContaining({ host: 'db.example.test', username: 'reader' }),
    )
    expect(result.current.scopeDiscovery?.scopes).toHaveLength(2)
    expect(result.current.defaultScope).toEqual([{ kind: 'database', name: 'analytics' }])

    act(() => result.current.selectDatabase('reporting'))
    expect(result.current.defaultScope).toEqual([{ kind: 'database', name: 'reporting' }])

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.default_scope).toEqual([{ kind: 'database', name: 'reporting' }])
  })

  it('hydrates ssh state and secret states on open', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          ssh_config: {
            enabled: true,
            host: 'bastion',
            port: 2222,
            user: 'jump',
            auth_method: 'password',
            known_hosts_entry: '',
            fingerprint: '',
            insecure_skip_host_key: true,
          },
          secrets: {
            password: { set: true, source: 'stored', revealable: true },
            ssh_password: { set: true, source: 'stored', revealable: true },
          },
        }),
      ),
    )
    const { result } = renderForm()

    await waitFor(() => expect(result.current.ssh.host).toBe('bastion'))
    expect(result.current.ssh.enabled).toBe(true)
    expect(result.current.ssh.port).toBe('2222')
    expect(result.current.bindSecret('ssh_password').state.kind).toBe('saved')
  })

  it('treats a secret supplied by an external source as managed and never sends it', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          secrets: { password: { set: true, source: 'vault', revealable: false } },
        }),
      ),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() =>
      expect(result.current.bindSecret('password').state).toEqual({
        kind: 'managed',
        source: 'vault',
      }),
    )

    act(() => result.current.bindSecret('password').dispatch({ type: 'edit', value: 'x' }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.secrets).toEqual({})
  })

  it('disables ssh by sending enabled false rather than a delete request', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          ssh_config: {
            enabled: true,
            host: 'bastion',
            port: 22,
            user: 'jump',
            auth_method: 'password',
            known_hosts_entry: '',
            fingerprint: '',
            insecure_skip_host_key: true,
          },
        }),
      ),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.ssh.enabled).toBe(true))

    act(() => result.current.changeSsh({ ...result.current.ssh, enabled: false }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect((patch.body.ssh_config as Record<string, unknown>).enabled).toBe(false)
  })

  it('sends a saved ssh secret clear when disabling the tunnel', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          ssh_config: {
            enabled: true,
            host: 'bastion',
            port: 22,
            user: 'jump',
            auth_method: 'password',
            known_hosts_entry: '',
            fingerprint: '',
            insecure_skip_host_key: true,
          },
          secrets: { ssh_password: { set: true, source: 'stored', revealable: true } },
        }),
      ),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.bindSecret('ssh_password').state.kind).toBe('saved'))

    act(() => result.current.bindSecret('ssh_password').dispatch({ type: 'clear' }))
    act(() => result.current.changeSsh({ ...result.current.ssh, enabled: false }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.secrets).toEqual({ ssh_password: null })
  })

  it('sends a saved ssh password clear when switching auth method', async () => {
    server.use(
      detailHandler(
        connectionDetailFixture({
          ssh_config: {
            enabled: true,
            host: 'bastion',
            port: 22,
            user: 'jump',
            auth_method: 'password',
            known_hosts_entry: '',
            fingerprint: '',
            insecure_skip_host_key: true,
          },
          secrets: { ssh_password: { set: true, source: 'stored', revealable: true } },
        }),
      ),
    )
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.bindSecret('ssh_password').state.kind).toBe('saved'))

    act(() => result.current.bindSecret('ssh_password').dispatch({ type: 'clear' }))
    act(() => result.current.changeSsh({ ...result.current.ssh, authMethod: 'private_key' }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.secrets).toEqual({ ssh_password: null })
  })

  it('hydrates show all databases and sends the edited value', async () => {
    stubEngine({ show_all_databases: true })
    const patch = capturePatch()
    const { result } = renderForm()
    await waitFor(() => expect(result.current.showAllDatabasesSupported).toBe(true))
    await waitFor(() => expect(result.current.requiredFieldsFilled).toBe(true))
    expect(result.current.showAllDatabases).toBe(true)

    act(() => result.current.selectDatabase('analytics'))
    expect(result.current.showAllDatabasesForced).toBe(false)
    act(() => result.current.changeShowAllDatabases(false))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patch.body.show_all_databases).toBe(false)
  })
})
