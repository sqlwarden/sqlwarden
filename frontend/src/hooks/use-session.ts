import { useQuery } from '@tanstack/react-query'
import { sessionQueryOptions } from '#/lib/api/query'
import { ApiError } from '#/lib/api/errors'

export function useSession(enabled = true) {
  return useQuery({
    ...sessionQueryOptions(),
    enabled,
    retry: (failureCount, error) => {
      if (error instanceof ApiError && error.status === 401) {
        return false
      }
      return failureCount < 1
    },
  })
}
