import { api } from '#/lib/api/client'
import type { Connection, TestConnectionResult } from '#/lib/types/workspace'

export const connectionsApi = {
  list: (orgSlug: string, wsId: string) =>
    api.get<Connection[]>(`/orgs/${orgSlug}/workspaces/${wsId}/connections`).then(r => r.data),
  create: (orgSlug: string, wsId: string, name: string, driver: string, dsn: string) =>
    api.post<Connection>(`/orgs/${orgSlug}/workspaces/${wsId}/connections`, { name, driver, dsn }).then(r => r.data),
  delete: (orgSlug: string, wsId: string, connId: string) =>
    api.delete(`/orgs/${orgSlug}/workspaces/${wsId}/connections/${connId}`),
  testNew: (orgSlug: string, wsId: string, driver: string, dsn: string) =>
    api.post<TestConnectionResult>(`/orgs/${orgSlug}/workspaces/${wsId}/connections/test`, { driver, dsn }).then(r => r.data),
}
