import { api } from '#/lib/api/client'
import type { Workspace } from '#/lib/types/workspace'
import type { AccessGrant } from '#/lib/types/role'

export const workspacesApi = {
  list: (orgSlug: string) =>
    api.get<Workspace[]>(`/orgs/${orgSlug}/workspaces`).then(r => r.data),
  get: (orgSlug: string, wsId: string) =>
    api.get<Workspace>(`/orgs/${orgSlug}/workspaces/${wsId}`).then(r => r.data),
  create: (orgSlug: string, name: string, description: string) =>
    api.post<Workspace>(`/orgs/${orgSlug}/workspaces`, { name, description }).then(r => r.data),
  update: (orgSlug: string, wsId: string, data: { name?: string; description?: string }) =>
    api.patch<Workspace>(`/orgs/${orgSlug}/workspaces/${wsId}`, data).then(r => r.data),
  listAccess: (orgSlug: string, wsId: string) =>
    api.get<AccessGrant[]>(`/orgs/${orgSlug}/workspaces/${wsId}/access`).then(r => r.data),
  grantAccess: (orgSlug: string, wsId: string, subject: string, action: string, expiresAt?: string) =>
    api.post(`/orgs/${orgSlug}/workspaces/${wsId}/access`, { subject, action, expires_at: expiresAt }),
  revokeAccess: (orgSlug: string, wsId: string, subject: string) =>
    api.delete(`/orgs/${orgSlug}/workspaces/${wsId}/access/${encodeURIComponent(subject)}`),
}
