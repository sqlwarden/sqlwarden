import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type { Account, InstanceAdmin, InstanceSettings, ListQuery, Organization, Paginated } from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

export function instanceOrganizationsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.instanceOrganizations(query),
    queryFn: () => api.get<Paginated<Organization>>('/api/v1/instance/orgs', { query }),
    placeholderData: keepPreviousData,
  })
}

export function instanceAccountsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.instanceAccounts(query),
    queryFn: () => api.get<Paginated<Account>>('/api/v1/instance/accounts', { query }),
    placeholderData: keepPreviousData,
  })
}

export function instanceAdminsQueryOptions(query?: ListQuery) {
  return queryOptions({
    queryKey: queryKeys.instanceAdmins(query),
    queryFn: () => api.get<Paginated<InstanceAdmin>>('/api/v1/instance/admins', { query }),
    placeholderData: keepPreviousData,
  })
}

export function instanceSettingsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.instanceSettings(),
    queryFn: () => api.get<InstanceSettings>('/api/v1/instance/settings'),
    staleTime: 60_000,
  })
}

