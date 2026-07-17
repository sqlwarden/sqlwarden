import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'
import { queryKeys } from '#/lib/api/query-keys'
import { OPTIONAL_FEATURES } from '#/lib/product/optional-features'
import { server } from '#/test/server'
import { useCapability } from './use-capability'

function wrapperWith(capabilities: string[] | undefined) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  if (capabilities) {
    client.setQueryData(queryKeys.instanceCapabilities(), { capabilities })
  }
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

describe('useCapability', () => {
  it('is active when the capability is available', () => {
    const { result } = renderHook(() => useCapability(OPTIONAL_FEATURES.auditLog.id), {
      wrapper: wrapperWith([OPTIONAL_FEATURES.auditLog.id]),
    })
    expect(result.current.state).toBe('active')
  })

  it('is locked when the capability is unavailable', () => {
    const { result } = renderHook(() => useCapability(OPTIONAL_FEATURES.auditLog.id), {
      wrapper: wrapperWith([]),
    })
    expect(result.current.state).toBe('locked')
  })

  it('reports loading before capability data arrives', () => {
    const { result } = renderHook(() => useCapability(OPTIONAL_FEATURES.auditLog.id), {
      wrapper: wrapperWith(undefined),
    })
    expect(result.current.state).toBe('loading')
  })

  it('reports an endpoint failure', async () => {
    server.use(
      http.get('/api/v1/instance/capabilities', () =>
        HttpResponse.json(
          { error: { code: 'unavailable', message: 'Unavailable' } },
          { status: 503 },
        ),
      ),
    )
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const Wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useCapability(OPTIONAL_FEATURES.auditLog.id), {
      wrapper: Wrapper,
    })

    await waitFor(() => expect(result.current.state).toBe('error'))
  })

  it('retains known access during a transient refresh failure', async () => {
    let attempts = 0
    server.use(
      http.get('/api/v1/instance/capabilities', () => {
        attempts++
        if (attempts > 1) {
          return HttpResponse.json(
            { error: { code: 'unavailable', message: 'Unavailable' } },
            { status: 503 },
          )
        }
        return HttpResponse.json({ capabilities: [OPTIONAL_FEATURES.auditLog.id] })
      }),
    )
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const Wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useCapability(OPTIONAL_FEATURES.auditLog.id), {
      wrapper: Wrapper,
    })
    await waitFor(() => expect(result.current.state).toBe('active'))

    act(() => result.current.retry())
    await waitFor(() => expect(attempts).toBe(2))
    expect(result.current.state).toBe('active')
  })
})
