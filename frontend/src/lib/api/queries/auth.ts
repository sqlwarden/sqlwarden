import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  AccountOrganization,
  ListQuery,
  Paginated,
  SessionResponse,
  SetupStatusResponse,
} from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

export function setupStatusQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.setupStatus(),
    queryFn: () => api.get<SetupStatusResponse>('/api/setup/status', { skipAuth: true }),
    staleTime: 60_000,
  })
}

export function sessionQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.session(),
    queryFn: () => api.get<SessionResponse>('/api/v1/session'),
    staleTime: 60_000,
  })
}

export function accountOrganizationsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.accountOrganizations(query),
    queryFn: () => api.get<Paginated<AccountOrganization>>('/api/v1/account/orgs', { query }),
    placeholderData: keepPreviousData,
  })
}
