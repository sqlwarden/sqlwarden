import { useQuery } from '@tanstack/react-query'
import { setupStatusQueryOptions } from '#/lib/api/query'

export function useSetupStatus() {
  return useQuery(setupStatusQueryOptions())
}
