import type { QueryClient } from '@tanstack/react-query'

// User-scoped data includes effective permissions, org membership, roles, and
// instance-admin state. It must not survive logout/login account switches.
export function clearAuthScopedQueryCache(queryClient: QueryClient) {
  queryClient.clear()
}
