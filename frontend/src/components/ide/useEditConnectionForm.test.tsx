import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Connection } from '#/lib/api/types'
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
  created_at: '',
  updated_at: '',
}

const disabledSshReveal = {
  enabled: false,
  host: '',
  port: 22,
  user: '',
  auth_method: 'password',
  known_hosts_entry: '',
  fingerprint: '',
  insecure_skip_host_key: false,
  password_set: false,
  private_key_set: false,
}

function sshRevealHandler(body: Record<string, unknown> = disabledSshReveal) {
  return http.get('/api/v1/orgs/acme/workspaces/3/connections/7/ssh', () => HttpResponse.json(body))
}

describe('useEditConnectionForm DSN reveal', () => {
  const queryClient = createTestQueryClient()
  let onOpenChange: ReturnType<typeof vi.fn>

  beforeEach(() => {
    queryClient.clear()
    onOpenChange = vi.fn()
    server.use(sshRevealHandler())
  })

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  function renderForm(canRevealDsn: boolean, initialOpen = true) {
    return renderHook(
      ({ open }: { open: boolean }) =>
        useEditConnectionForm({
          open,
          onOpenChange,
          orgSlug: 'acme',
          workspaceId: 3,
          connection,
          canRevealDsn,
        }),
      { wrapper, initialProps: { open: initialOpen } },
    )
  }

  it('fetches and parses the DSN into fields when canRevealDsn is true', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({
          dsn: 'postgresql://reader:secret@db.internal:5433/analytics',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'disable',
          server_name: '',
          ca_pem: '',
          client_cert_pem: '',
          client_key_set: false,
        }),
      ),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.fields.host).toBe('db.internal'))
    expect(result.current.fields).toEqual(
      expect.objectContaining({
        host: 'db.internal',
        port: '5433',
        database: 'analytics',
        username: 'reader',
        password: 'secret',
      }),
    )
  })

  it('hydrates TLS state from the reveal endpoint and sends it in the update payload', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'verify-ca',
          server_name: 'db.internal',
          ca_pem: '-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----',
          client_cert_pem: '',
          client_key_set: true,
        }),
      ),
    )
    let body: Record<string, unknown> = {}
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/connections/7', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 })
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.tls.mode).toBe('verify-ca'))
    expect(result.current.tls.serverName).toBe('db.internal')
    expect(result.current.tls.clientKeySet).toBe(true)
    expect(result.current.tls.clientKeyPem).toBe('')

    act(() => {
      result.current.changeName('analytics-pg')
      for (const field of result.current.driver.fields.filter((f) => f.required)) {
        result.current.changeField(field.key, field.default ?? `${field.key}-value`)
      }
    })
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.tls).toEqual({
      mode: 'verify-ca',
      server_name: 'db.internal',
      ca_pem: '-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----',
      client_cert_pem: '',
      client_key_pem: '',
      clear_client_key: false,
    })
  })

  it('sends clear_client_key when the stored TLS client key is removed', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          configured: true,
          mode: 'verify-full',
          server_name: 'db.internal',
          ca_pem: '',
          client_cert_pem: '-----BEGIN CERTIFICATE-----\ncrt\n-----END CERTIFICATE-----',
          client_key_set: true,
        }),
      ),
    )
    let body: Record<string, unknown> = {}
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/connections/7', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 })
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.tls.clientKeySet).toBe(true))
    act(() => result.current.changeTls({ ...result.current.tls, clearClientKey: true }))
    act(() => {
      result.current.changeName('analytics-pg')
      for (const field of result.current.driver.fields.filter((f) => f.required)) {
        result.current.changeField(field.key, field.default ?? `${field.key}-value`)
      }
    })
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect((body.tls as Record<string, unknown>).clear_client_key).toBe(true)
  })

  it('does not fetch the DSN when canRevealDsn is false', async () => {
    let requested = false
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () => {
        requested = true
        return HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' })
      }),
    )
    const { result } = renderForm(false)

    await waitFor(() => expect(result.current.revealDsnPending).toBe(false))
    expect(requested).toBe(false)
    expect(result.current.fields.host).toBe('')
  })

  it('does not fetch the DSN when the org masks credentials on edit, even with canRevealDsn true', async () => {
    let requested = false
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: true,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () => {
        requested = true
        return HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' })
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.revealDsnAllowed).toBe(false))
    expect(requested).toBe(false)
    expect(result.current.fields.host).toBe('')
  })

  it('re-populates the form when the same connection is edited again after closing', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({
          dsn: 'postgresql://reader:secret@db.internal:5433/analytics',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'disable',
          server_name: '',
          ca_pem: '',
          client_cert_pem: '',
          client_key_set: false,
        }),
      ),
    )
    const { result, rerender } = renderForm(true)
    await waitFor(() => expect(result.current.fields.host).toBe('db.internal'))

    act(() => result.current.handleOpenChange(false))
    rerender({ open: false })
    expect(result.current.fields.host).toBe('')

    rerender({ open: true })
    await waitFor(() => expect(result.current.fields.host).toBe('db.internal'))
  })

  it('discovers scopes on test and includes the selected default_scope in the update payload', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections/test', () =>
        HttpResponse.json({
          ok: true,
          latency_ms: 5,
          scope_discovery: {
            current: [{ kind: 'database', name: 'analytics' }],
            scopes: [
              [{ kind: 'database', name: 'analytics' }],
              [{ kind: 'database', name: 'reporting' }],
            ],
          },
        }),
      ),
    )
    let body: Record<string, unknown> = {}
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/connections/7', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 })
      }),
    )
    const { result } = renderForm(false)
    await waitFor(() => expect(result.current.revealDsnPending).toBe(false))
    act(() => {
      result.current.changeName('analytics-pg')
      for (const field of result.current.driver.fields.filter((f) => f.required)) {
        result.current.changeField(field.key, field.default ?? `${field.key}-value`)
      }
    })

    await act(() => result.current.testConnection.mutateAsync())
    expect(result.current.scopeDiscovery?.scopes).toHaveLength(2)
    expect(result.current.defaultScope).toEqual([{ kind: 'database', name: 'analytics' }])

    act(() => result.current.selectDatabase('reporting'))
    expect(result.current.defaultScope).toEqual([{ kind: 'database', name: 'reporting' }])

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body.default_scope).toEqual([{ kind: 'database', name: 'reporting' }])
  })

  it('hydrates ssh state from the reveal query on open', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'disable',
          server_name: '',
          ca_pem: '',
          client_cert_pem: '',
          client_key_set: false,
        }),
      ),
      sshRevealHandler({
        enabled: true,
        host: 'bastion',
        port: 2222,
        user: 'jump',
        auth_method: 'password',
        known_hosts_entry: '',
        fingerprint: '',
        insecure_skip_host_key: true,
        password_set: true,
        private_key_set: false,
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.ssh.host).toBe('bastion'))
    expect(result.current.ssh.enabled).toBe(true)
    expect(result.current.ssh.port).toBe('2222')
    expect(result.current.ssh.passwordSet).toBe(true)
    expect(result.current.ssh.password).toBe('')
  })

  it('carries the stored key forward by sending blank secrets', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'disable',
          server_name: '',
          ca_pem: '',
          client_cert_pem: '',
          client_key_set: false,
        }),
      ),
      sshRevealHandler({
        enabled: true,
        host: 'bastion',
        port: 22,
        user: 'jump',
        auth_method: 'private_key',
        known_hosts_entry: '',
        fingerprint: '',
        insecure_skip_host_key: true,
        password_set: false,
        private_key_set: true,
      }),
    )
    let body: Record<string, unknown> = {}
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/connections/7', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 })
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.ssh.host).toBe('bastion'))

    act(() => {
      result.current.changeName('analytics-pg')
      for (const field of result.current.driver.fields.filter((f) => f.required)) {
        result.current.changeField(field.key, field.default ?? `${field.key}-value`)
      }
    })
    act(() => result.current.changeSsh({ ...result.current.ssh, host: 'bastion-2' }))
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    const ssh = body.ssh as Record<string, unknown>
    expect(ssh.host).toBe('bastion-2')
    expect(ssh.private_key_pem).toBe('')
    expect(ssh.passphrase).toBe('')
    expect(ssh.enabled).toBe(true)
  })

  it('sends clear_private_key (and clear_passphrase) when the stored key is removed', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'disable',
          server_name: '',
          ca_pem: '',
          client_cert_pem: '',
          client_key_set: false,
        }),
      ),
      sshRevealHandler({
        configured: true,
        enabled: true,
        host: 'bastion',
        port: 22,
        user: 'jump',
        auth_method: 'private_key',
        known_hosts_entry: '',
        fingerprint: '',
        insecure_skip_host_key: true,
        password_set: false,
        private_key_set: true,
      }),
    )
    let body: Record<string, unknown> = {}
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/connections/7', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 7 })
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.ssh.privateKeySet).toBe(true))
    act(() => result.current.changeSsh({ ...result.current.ssh, clearPrivateKey: true }))
    act(() => {
      result.current.changeName('analytics-pg')
      for (const field of result.current.driver.fields.filter((f) => f.required)) {
        result.current.changeField(field.key, field.default ?? `${field.key}-value`)
      }
    })
    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    const ssh = body.ssh as Record<string, unknown>
    expect(ssh.clear_private_key).toBe(true)
    expect(ssh.clear_passphrase).toBe(true)
    expect(ssh.clear_password).toBe(false)
  })

  it('removeSsh calls the SSH delete endpoint and resets ssh state', async () => {
    server.use(
      http.get('/api/v1/orgs/acme', () =>
        HttpResponse.json({
          id: 1,
          slug: 'acme',
          name: 'Acme',
          mask_connection_credentials_on_edit: false,
          created_at: '',
          updated_at: '',
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/dsn', () =>
        HttpResponse.json({ dsn: 'postgresql://reader:secret@db.internal:5433/analytics' }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/tls', () =>
        HttpResponse.json({
          mode: 'disable',
          server_name: '',
          ca_pem: '',
          client_cert_pem: '',
          client_key_set: false,
        }),
      ),
      sshRevealHandler({
        configured: true,
        enabled: true,
        host: 'bastion',
        port: 22,
        user: 'jump',
        auth_method: 'password',
        known_hosts_entry: '',
        fingerprint: '',
        insecure_skip_host_key: true,
        password_set: true,
        private_key_set: false,
      }),
    )
    let deleted = false
    server.use(
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/7/ssh', () => {
        deleted = true
        server.use(sshRevealHandler({ ...disabledSshReveal, configured: false }))
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { result } = renderForm(true)

    await waitFor(() => expect(result.current.sshConfigured).toBe(true))
    await act(async () => {
      await result.current.removeSsh.mutateAsync()
    })

    expect(deleted).toBe(true)
    await waitFor(() => expect(result.current.sshConfigured).toBe(false))
    expect(result.current.ssh.host).toBe('')
    expect(result.current.ssh.enabled).toBe(false)
  })
})
