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
  created_at: '',
  updated_at: '',
}

describe('useEditConnectionForm DSN reveal', () => {
  const queryClient = createTestQueryClient()
  let onOpenChange: ReturnType<typeof vi.fn>

  beforeEach(() => {
    queryClient.clear()
    onOpenChange = vi.fn()
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
          dsn: 'postgresql://reader:secret@db.internal:5433/analytics?sslmode=require',
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
        sslmode: 'require',
      }),
    )
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
})
