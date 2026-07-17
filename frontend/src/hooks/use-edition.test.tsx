import { describe, expect, it } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import type { InstanceEdition } from '#/lib/api/types'
import { ENTERPRISE_FEATURES } from '#/lib/enterprise/features'
import { server } from '#/test/server'
import { useFeature } from './use-edition'

function wrapperWith(edition: InstanceEdition | undefined) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  if (edition) {
    client.setQueryData(queryKeys.instanceEdition(), edition)
  }
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

describe('useFeature', () => {
  it('is active when the feature is licensed', () => {
    const { result } = renderHook(() => useFeature(ENTERPRISE_FEATURES.auditLog), {
      wrapper: wrapperWith({
        edition: 'enterprise',
        licensed_features: [ENTERPRISE_FEATURES.auditLog],
      }),
    })
    expect(result.current.state).toBe('active')
  })

  it('is locked on an unlicensed enterprise server', () => {
    const { result } = renderHook(() => useFeature(ENTERPRISE_FEATURES.auditLog), {
      wrapper: wrapperWith({ edition: 'enterprise', licensed_features: [] }),
    })
    expect(result.current.state).toBe('locked')
  })

  it('is unavailable only after a community response loads', () => {
    const { result } = renderHook(() => useFeature(ENTERPRISE_FEATURES.auditLog), {
      wrapper: wrapperWith({ edition: 'community', licensed_features: [] }),
    })
    expect(result.current.state).toBe('unavailable')
  })

  it('reports loading before edition data arrives', () => {
    const { result } = renderHook(() => useFeature(ENTERPRISE_FEATURES.auditLog), {
      wrapper: wrapperWith(undefined),
    })
    expect(result.current.state).toBe('loading')
  })

  it('reports an endpoint failure instead of treating it as community', async () => {
    server.use(
      http.get('/api/v1/instance/edition', () =>
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
    const { result } = renderHook(() => useFeature(ENTERPRISE_FEATURES.auditLog), {
      wrapper: Wrapper,
    })

    await waitFor(() => expect(result.current.state).toBe('error'))
  })

  it('retains known access during a transient refresh failure', async () => {
    let attempts = 0
    server.use(
      http.get('/api/v1/instance/edition', () => {
        attempts++
        if (attempts > 1) {
          return HttpResponse.json(
            { error: { code: 'unavailable', message: 'Unavailable' } },
            { status: 503 },
          )
        }
        return HttpResponse.json({
          edition: 'enterprise',
          licensed_features: [ENTERPRISE_FEATURES.auditLog],
        })
      }),
    )
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const Wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useFeature(ENTERPRISE_FEATURES.auditLog), {
      wrapper: Wrapper,
    })
    await waitFor(() => expect(result.current.state).toBe('active'))

    act(() => result.current.retry())
    await waitFor(() => expect(attempts).toBe(2))
    expect(result.current.state).toBe('active')
  })
})
