import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
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

  function renderForm(canRevealDsn: boolean) {
    return renderHook(
      () =>
        useEditConnectionForm({
          open: true,
          onOpenChange,
          orgSlug: 'acme',
          workspaceId: 3,
          connection,
          canRevealDsn,
        }),
      { wrapper },
    )
  }

  it('fetches and parses the DSN into fields when canRevealDsn is true', async () => {
    server.use(
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
})
