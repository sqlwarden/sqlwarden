import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Environment } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
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

describe('useConnectionForm', () => {
  const queryClient = createTestQueryClient()
  let onOpenChange: ReturnType<typeof vi.fn>

  beforeEach(() => {
    queryClient.clear()
    onOpenChange = vi.fn()
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

  function fillRequiredFields(result: ReturnType<typeof renderForm>['result']) {
    act(() => {
      result.current.changeName('Warehouse')
      for (const field of result.current.currentDriver.fields.filter(
        (candidate) => candidate.required,
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

  it('validates connection name, environment, and every required driver field', () => {
    const { result } = renderForm({ environments: [] })
    act(() => result.current.pickDriver(drivers[0].id))

    act(() => result.current.submit())

    expect(result.current.errors.name).toBe('Name is required.')
    expect(result.current.errors.environmentId).toBe('Environment is required.')
    for (const field of result.current.currentDriver.fields.filter(
      (candidate) => candidate.required && !candidate.default,
    )) {
      expect(result.current.errors.fields[field.key]).toBe(`${field.label} is required.`)
    }
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
    expect(result.current.fields).toEqual(
      expect.objectContaining(
        Object.fromEntries(alternate!.fields.map((field) => [field.key, field.default ?? ''])),
      ),
    )
    expect(result.current.testState).toEqual({ status: 'idle' })
  })

  it('creates a validated connection with the driver DSN and invalidates the list', async () => {
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
    fillRequiredFields(result)
    const expectedDsn = drivers[0].buildDSN(result.current.fields)

    act(() => result.current.submit())

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(body).toEqual(
      expect.objectContaining({
        name: 'Warehouse',
        driver: drivers[0].id,
        environment_id: 4,
        access_mode: 'open',
        dsn: expectedDsn,
      }),
    )
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['org-workspace-connections', 'acme', 3] })
  })
})
