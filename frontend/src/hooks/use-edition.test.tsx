import { describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import type { InstanceEdition } from '#/lib/api/types'
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
    const { result } = renderHook(() => useFeature('audit_log'), {
      wrapper: wrapperWith({ edition: 'enterprise', licensed_features: ['audit_log'] }),
    })
    expect(result.current).toBe('active')
  })

  it('is locked on an unlicensed enterprise server', () => {
    const { result } = renderHook(() => useFeature('audit_log'), {
      wrapper: wrapperWith({ edition: 'enterprise', licensed_features: [] }),
    })
    expect(result.current).toBe('locked')
  })

  it('is unavailable on community servers and before data loads', () => {
    const { result } = renderHook(() => useFeature('audit_log'), {
      wrapper: wrapperWith({ edition: 'community', licensed_features: [] }),
    })
    expect(result.current).toBe('unavailable')

    const { result: pending } = renderHook(() => useFeature('audit_log'), {
      wrapper: wrapperWith(undefined),
    })
    expect(pending.current).toBe('unavailable')
  })
})
