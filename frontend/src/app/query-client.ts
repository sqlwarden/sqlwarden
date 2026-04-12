import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '#/lib/api/errors'

function shouldRetry(failureCount: number, error: unknown) {
  if (error instanceof ApiError) {
    if ([400, 401, 403, 404, 409, 422].includes(error.status)) {
      return false
    }
  }

  return failureCount < 2
}

export function createAppQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: shouldRetry,
        refetchOnWindowFocus: false,
        staleTime: 30_000,
      },
      mutations: {
        retry: false,
      },
    },
  })
}
