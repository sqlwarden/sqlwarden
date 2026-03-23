import { api } from '#/lib/api/client'
import type { Tenant, TenantMemberWithAccount, OrgAuthInfo } from '#/lib/types/org'
import type { Team, TeamMemberWithAccount } from '#/lib/types/team'
import type { WorkspaceRoleWithActions } from '#/lib/types/role'

export const orgsApi = {
  getOrgAuthInfo: (slug: string) =>
    api.get<OrgAuthInfo>(`/orgs/${slug}/auth-info`).then(r => r.data),
  getOrg: (slug: string) =>
    api.get<Tenant>(`/orgs/${slug}`).then(r => r.data),
  updateOrg: (slug: string, name: string) =>
    api.patch<Tenant>(`/orgs/${slug}`, { name }).then(r => r.data),
  listMembers: (slug: string) =>
    api.get<TenantMemberWithAccount[]>(`/orgs/${slug}/members`).then(r => r.data),
  inviteMember: (slug: string, email: string, role: string) =>
    api.post(`/orgs/${slug}/members`, { email, role }),
  updateMemberRole: (slug: string, accountId: string, role: string) =>
    api.patch(`/orgs/${slug}/members/${accountId}`, { role }),
  removeMember: (slug: string, accountId: string) =>
    api.delete(`/orgs/${slug}/members/${accountId}`),
  listTeams: (slug: string) =>
    api.get<Team[]>(`/orgs/${slug}/teams`).then(r => r.data),
  createTeam: (slug: string, teamSlug: string, name: string) =>
    api.post<Team>(`/orgs/${slug}/teams`, { slug: teamSlug, name }).then(r => r.data),
  deleteTeam: (slug: string, teamSlug: string) =>
    api.delete(`/orgs/${slug}/teams/${teamSlug}`),
  listTeamMembers: (slug: string, teamSlug: string) =>
    api.get<TeamMemberWithAccount[]>(`/orgs/${slug}/teams/${teamSlug}/members`).then(r => r.data),
  addTeamMember: (slug: string, teamSlug: string, accountId: string) =>
    api.post(`/orgs/${slug}/teams/${teamSlug}/members`, { account_id: accountId }),
  removeTeamMember: (slug: string, teamSlug: string, accountId: string) =>
    api.delete(`/orgs/${slug}/teams/${teamSlug}/members/${accountId}`),
  listRoles: (slug: string) =>
    api.get<WorkspaceRoleWithActions[]>(`/orgs/${slug}/roles`).then(r => r.data),
  createRole: (slug: string, name: string, description: string, actions: string[]) =>
    api.post<WorkspaceRoleWithActions>(`/orgs/${slug}/roles`, { name, description, actions }).then(r => r.data),
  deleteRole: (slug: string, roleId: string) =>
    api.delete(`/orgs/${slug}/roles/${roleId}`),
  getUserOrgs: () =>
    api.get<Tenant[]>('/user/orgs').then(r => r.data),
}
