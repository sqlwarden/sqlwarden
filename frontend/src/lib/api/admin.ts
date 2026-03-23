import { api } from '#/lib/api/client'
import type { Tenant } from '#/lib/types/org'
import type { Account } from '#/lib/types/auth'
import type { InstanceSettings, PaginatedResponse } from '#/lib/types/admin'

export const adminApi = {
  listOrgs: (page = 1, limit = 50) =>
    api.get<PaginatedResponse<Tenant>>('/admin/orgs', { params: { page, limit } }).then(r => r.data),
  createOrg: (slug: string, name: string, ownerEmail: string) =>
    api.post<Tenant>('/admin/orgs', { slug, name, owner_email: ownerEmail }).then(r => r.data),
  listAccounts: (page = 1, limit = 50) =>
    api.get<PaginatedResponse<Account>>('/admin/accounts', { params: { page, limit } }).then(r => r.data),
  getSettings: () =>
    api.get<InstanceSettings>('/admin/settings').then(r => r.data),
  updateSetting: (key: string, value: string) =>
    api.patch('/admin/settings', { key, value }),
}
