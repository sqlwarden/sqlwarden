import { useQuery } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import { orgsApi } from '#/lib/api/orgs'
import type { Account } from '#/lib/types/auth'

export const authKeys = {
  user: ['user'] as const,
  orgs: ['user', 'orgs'] as const,
}

export function useCurrentUser() {
  return useQuery({
    queryKey: authKeys.user,
    queryFn: () => api.get<Account>('/user').then(r => r.data),
    staleTime: 5 * 60 * 1000,
  })
}

export function useUserOrgs() {
  return useQuery({
    queryKey: authKeys.orgs,
    queryFn: () => orgsApi.getUserOrgs(),
    staleTime: 2 * 60 * 1000,
  })
}
