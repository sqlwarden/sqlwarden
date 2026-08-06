import type { ListQuery, ObjectRef, ResourceType } from '#/lib/api/types'

/**
 * Canonical TanStack Query keys. Scope keys intentionally omit list filters so
 * mutations can invalidate every cached variant of a resource collection.
 */
export const queryKeys = {
  setupStatus: () => ['setup-status'] as const,
  session: () => ['session'] as const,
  accountOrganizationsScope: () => ['account-organizations'] as const,
  accountOrganizations: (query?: ListQuery) =>
    [...queryKeys.accountOrganizationsScope(), query ?? {}] as const,
  instanceAccountsScope: () => ['instance-accounts'] as const,
  instanceAccounts: (query?: ListQuery) =>
    [...queryKeys.instanceAccountsScope(), query ?? {}] as const,
  instanceAdminsScope: () => ['instance-admins'] as const,
  instanceAdmins: (query?: ListQuery) => [...queryKeys.instanceAdminsScope(), query ?? {}] as const,
  instanceOrganizationsScope: () => ['instance-organizations'] as const,
  instanceOrganizations: (query?: ListQuery) =>
    [...queryKeys.instanceOrganizationsScope(), query ?? {}] as const,
  instanceSettings: () => ['instance-settings'] as const,
  instanceConfiguration: () => ['instance-configuration'] as const,
  orgEffectivePermissions: (
    slug: string,
    resourceType: ResourceType,
    resourceId?: string | number,
  ) => ['org-effective-permissions', slug, resourceType, resourceId ?? null] as const,
  orgPermissions: (slug: string) => ['org-permissions', slug] as const,
  org: (slug: string) => ['org', slug] as const,
  orgRuntimeSettings: (slug: string) => ['org-runtime-settings', slug] as const,
  orgMembersScope: (slug: string) => ['org-members', slug] as const,
  orgMembers: (slug: string, query?: ListQuery) =>
    [...queryKeys.orgMembersScope(slug), query ?? {}] as const,
  orgInvitationsScope: (slug: string) => ['org-invitations', slug] as const,
  orgInvitations: (slug: string, query?: ListQuery) =>
    [...queryKeys.orgInvitationsScope(slug), query ?? {}] as const,
  invitation: (token: string) => ['invitation', token] as const,
  orgMember: (slug: string, accountId: string | number) => ['org-member', slug, accountId] as const,
  orgMemberTeamsScope: (slug: string, accountId: string | number) =>
    ['org-member-teams', slug, accountId] as const,
  orgMemberTeams: (slug: string, accountId: string | number, query?: ListQuery) =>
    [...queryKeys.orgMemberTeamsScope(slug, accountId), query ?? {}] as const,
  orgTeamsScope: (slug: string) => ['org-teams', slug] as const,
  orgTeams: (slug: string, query?: ListQuery) =>
    [...queryKeys.orgTeamsScope(slug), query ?? {}] as const,
  orgTeam: (slug: string, teamSlug: string) => ['org-team', slug, teamSlug] as const,
  orgTeamMembersScope: (slug: string, teamSlug: string) =>
    ['org-team-members', slug, teamSlug] as const,
  orgTeamMembers: (slug: string, teamSlug: string, query?: ListQuery) =>
    [...queryKeys.orgTeamMembersScope(slug, teamSlug), query ?? {}] as const,
  orgRolesScope: (slug: string) => ['org-roles', slug] as const,
  orgRoles: (slug: string, query?: ListQuery) =>
    [...queryKeys.orgRolesScope(slug), query ?? {}] as const,
  orgRole: (slug: string, roleId: string | number) => ['org-role', slug, roleId] as const,
  orgWorkspaceRolesScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-roles', slug, workspaceId] as const,
  orgWorkspaceRoles: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspaceRolesScope(slug, workspaceId), query ?? {}] as const,
  orgWorkspaceRole: (slug: string, workspaceId: string | number, roleId: string | number) =>
    ['org-workspace-role', slug, workspaceId, roleId] as const,
  orgWorkspacesScope: (slug: string) => ['org-workspaces', slug] as const,
  orgWorkspaces: (slug: string, query?: ListQuery) =>
    [...queryKeys.orgWorkspacesScope(slug), query ?? {}] as const,
  orgWorkspace: (slug: string, workspaceId: string | number) =>
    ['org-workspace', slug, workspaceId] as const,
  orgWorkspaceMembersScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-members', slug, workspaceId] as const,
  orgWorkspaceMembers: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspaceMembersScope(slug, workspaceId), query ?? {}] as const,
  orgWorkspaceEffectiveMembersScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-effective-members', slug, workspaceId] as const,
  orgWorkspaceEffectiveMembers: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspaceEffectiveMembersScope(slug, workspaceId), query ?? {}] as const,
  orgWorkspaceTeamsScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-teams', slug, workspaceId] as const,
  orgWorkspaceTeams: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspaceTeamsScope(slug, workspaceId), query ?? {}] as const,
  orgPoliciesScope: (slug: string) => ['org-policies', slug] as const,
  orgPolicies: (slug: string, query?: ListQuery) =>
    [...queryKeys.orgPoliciesScope(slug), query ?? {}] as const,
  orgPolicy: (slug: string, bindingId: string | number) => ['org-policy', slug, bindingId] as const,
  orgWorkspacePoliciesScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-policies', slug, workspaceId] as const,
  orgWorkspacePolicies: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspacePoliciesScope(slug, workspaceId), query ?? {}] as const,
  orgWorkspacePrivateFilesScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-private-files', slug, workspaceId] as const,
  orgWorkspacePrivateFiles: (
    slug: string,
    workspaceId: string | number,
    parentId?: string | number | null,
  ) => [...queryKeys.orgWorkspacePrivateFilesScope(slug, workspaceId), parentId ?? null] as const,
  orgWorkspaceSharedFilesScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-shared-files', slug, workspaceId] as const,
  orgWorkspaceSharedFiles: (
    slug: string,
    workspaceId: string | number,
    parentId?: string | number | null,
  ) => [...queryKeys.orgWorkspaceSharedFilesScope(slug, workspaceId), parentId ?? null] as const,
  orgWorkspacePrivateFileBrowserScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-private-file-browser', slug, workspaceId] as const,
  orgWorkspacePrivateFileBrowser: (
    slug: string,
    workspaceId: string | number,
    fileId?: string | number | null,
  ) =>
    [...queryKeys.orgWorkspacePrivateFileBrowserScope(slug, workspaceId), fileId ?? null] as const,
  orgWorkspaceSharedFileBrowserScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-shared-file-browser', slug, workspaceId] as const,
  orgWorkspaceSharedFileBrowser: (
    slug: string,
    workspaceId: string | number,
    fileId?: string | number | null,
  ) =>
    [...queryKeys.orgWorkspaceSharedFileBrowserScope(slug, workspaceId), fileId ?? null] as const,
  orgWorkspacePrivateRecentFiles: (slug: string, workspaceId: string | number, limit?: number) =>
    ['org-workspace-private-recent-files', slug, workspaceId, limit ?? null] as const,
  orgWorkspaceSharedRecentFiles: (slug: string, workspaceId: string | number, limit?: number) =>
    ['org-workspace-shared-recent-files', slug, workspaceId, limit ?? null] as const,
  fileContent: (slug: string, workspaceId: string | number, fileId: string | number | undefined) =>
    ['file-content', slug, workspaceId, fileId] as const,
  myWorkspaces: (query?: ListQuery) => ['my-workspaces', query ?? {}] as const,
  myWorkspacePrivateFiles: (workspaceId: string | number, parentId?: string | number | null) =>
    ['my-workspace-private-files', workspaceId, parentId ?? null] as const,
  myWorkspacePrivateFileBrowser: (workspaceId: string | number, fileId?: string | number | null) =>
    ['my-workspace-private-file-browser', workspaceId, fileId ?? null] as const,
  myWorkspacePrivateRecentFiles: (workspaceId: string | number, limit?: number) =>
    ['my-workspace-private-recent-files', workspaceId, limit ?? null] as const,
  orgEnvironmentsScope: (slug: string, workspaceId?: string | number) =>
    workspaceId === undefined
      ? (['org-environments', slug] as const)
      : (['org-environments', slug, workspaceId] as const),
  orgEnvironments: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgEnvironmentsScope(slug, workspaceId), query ?? {}] as const,
  myEnvironments: (workspaceId: string | number, query?: ListQuery) =>
    ['my-environments', workspaceId, query ?? {}] as const,
  orgConnections: (
    slug: string,
    workspaceId: string | number,
    environmentId: string | number,
    query?: ListQuery,
  ) => ['org-connections', slug, workspaceId, environmentId, query ?? {}] as const,
  orgWorkspaceConnectionsScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-connections', slug, workspaceId] as const,
  orgWorkspaceConnections: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspaceConnectionsScope(slug, workspaceId), query ?? {}] as const,
  myConnections: (
    workspaceId: string | number,
    environmentId: string | number,
    query?: ListQuery,
  ) => ['my-connections', workspaceId, environmentId, query ?? {}] as const,
  orgWorkspaceJobsScope: (slug: string, workspaceId: string | number) =>
    ['org-workspace-jobs', slug, workspaceId] as const,
  orgWorkspaceJobs: (slug: string, workspaceId: string | number, query?: ListQuery) =>
    [...queryKeys.orgWorkspaceJobsScope(slug, workspaceId), query ?? {}] as const,
  workspaceSessions: (slug: string, workspaceId: string | number) =>
    ['org-workspace-sessions', slug, workspaceId] as const,
  exportJobLog: (slug: string, workspaceId: string | number, jobId: string) =>
    ['export-job-log', slug, workspaceId, jobId] as const,
  exportJobLatestEvent: (slug: string, workspaceId: string | number, jobId: string) =>
    ['export-job-latest-event', slug, workspaceId, jobId] as const,
  connectionDsn: (slug: string, workspaceId: string | number, connectionId: string | number) =>
    ['connection-dsn', slug, workspaceId, connectionId] as const,
  connectionPreviewCount: (
    slug: string,
    workspaceId: string | number,
    connectionId: string | number,
    ref: ObjectRef,
  ) =>
    [
      'connection-preview-count',
      slug,
      workspaceId,
      connectionId,
      JSON.stringify(ref.scope),
      ref.kind,
      ref.name,
    ] as const,
}
