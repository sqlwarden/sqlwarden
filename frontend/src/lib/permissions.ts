// Mirrors internal/access/permissions.go. Keep names and groupings aligned with
// the backend permission model so UI capability checks use backend vocabulary.
import type { PermissionDefinition } from '#/lib/api/types'

export const permission = {
  orgRead: 'org:read',
  orgWrite: 'org:write',
  orgDelete: 'org:delete',
  orgInvite: 'org:invite',
  orgAssignRoles: 'org:assign_roles',
  orgTransferOwnership: 'org:transfer_ownership',

  wsRead: 'ws:read',
  wsWrite: 'ws:write',
  wsCreate: 'ws:create',
  wsDelete: 'ws:delete',

  envRead: 'env:read',
  envWrite: 'env:write',
  envCreate: 'env:create',
  envDelete: 'env:delete',
  envDeploy: 'env:deploy',

  connRead: 'conn:read',
  connWrite: 'conn:write',
  connCreate: 'conn:create',
  connDelete: 'conn:delete',
  connExecute: 'conn:execute',
  connDql: 'conn:dql',
  connDml: 'conn:dml',
  connDdl: 'conn:ddl',

  policyRead: 'policy:read',
  policyModify: 'policy:modify',
} as const

export type Permission = (typeof permission)[keyof typeof permission]
export type PermissionScope = 'org' | 'workspace' | 'environment' | 'connection'

export const builtinRole = {
  organizationOwner: 'Organization Owner',
  organizationAdmin: 'Organization Admin',
  organizationMember: 'Organization Member',
  workspaceAdmin: 'Workspace Admin',
  workspaceMember: 'Workspace Member',
} as const

export type BuiltinRole = (typeof builtinRole)[keyof typeof builtinRole]
export type OrgBuiltinRole =
  | typeof builtinRole.organizationOwner
  | typeof builtinRole.organizationAdmin
  | typeof builtinRole.organizationMember
export type WorkspaceBuiltinRole = typeof builtinRole.workspaceAdmin | typeof builtinRole.workspaceMember

export const scopePermissions = {
  org: [
    permission.orgRead,
    permission.orgWrite,
    permission.orgDelete,
    permission.orgInvite,
    permission.orgAssignRoles,
    permission.orgTransferOwnership,
    permission.wsRead,
    permission.wsWrite,
    permission.wsCreate,
    permission.wsDelete,
    permission.envRead,
    permission.envWrite,
    permission.envCreate,
    permission.envDelete,
    permission.envDeploy,
    permission.connRead,
    permission.connWrite,
    permission.connCreate,
    permission.connDelete,
    permission.connExecute,
    permission.connDql,
    permission.connDml,
    permission.connDdl,
    permission.policyRead,
    permission.policyModify,
  ],
  workspace: [
    permission.wsRead,
    permission.wsWrite,
    permission.envRead,
    permission.envWrite,
    permission.envCreate,
    permission.envDelete,
    permission.envDeploy,
    permission.connRead,
    permission.connWrite,
    permission.connCreate,
    permission.connDelete,
    permission.connExecute,
    permission.connDql,
    permission.connDml,
    permission.connDdl,
    permission.policyRead,
    permission.policyModify,
  ],
  environment: [
    permission.envRead,
    permission.envWrite,
    permission.envDelete,
    permission.envDeploy,
    permission.connRead,
    permission.connWrite,
    permission.connCreate,
    permission.connDelete,
    permission.connExecute,
    permission.connDql,
    permission.connDml,
    permission.connDdl,
  ],
  connection: [
    permission.connRead,
    permission.connWrite,
    permission.connDelete,
    permission.connExecute,
    permission.connDql,
    permission.connDml,
    permission.connDdl,
  ],
} as const satisfies Record<PermissionScope, readonly Permission[]>

export const orgBuiltinRoles = {
  [builtinRole.organizationOwner]: scopePermissions.org,
  [builtinRole.organizationAdmin]: [
    permission.orgRead,
    permission.orgWrite,
    permission.orgInvite,
    permission.orgAssignRoles,
    permission.wsCreate,
    permission.wsDelete,
    permission.wsRead,
    permission.wsWrite,
    permission.envRead,
    permission.envWrite,
    permission.envCreate,
    permission.envDelete,
    permission.envDeploy,
    permission.connRead,
    permission.connWrite,
    permission.connCreate,
    permission.connDelete,
    permission.connExecute,
    permission.connDql,
    permission.connDml,
    permission.connDdl,
    permission.policyRead,
    permission.policyModify,
  ],
  [builtinRole.organizationMember]: [
    permission.orgRead,
  ],
} as const satisfies Record<OrgBuiltinRole, readonly Permission[]>

export const workspaceBuiltinRoles = {
  [builtinRole.workspaceAdmin]: [
    permission.wsRead,
    permission.wsWrite,
    permission.envRead,
    permission.envWrite,
    permission.envCreate,
    permission.envDelete,
    permission.envDeploy,
    permission.connRead,
    permission.connWrite,
    permission.connCreate,
    permission.connDelete,
    permission.connExecute,
    permission.connDql,
    permission.connDml,
    permission.connDdl,
    permission.policyRead,
    permission.policyModify,
  ],
  [builtinRole.workspaceMember]: [
    permission.wsRead,
  ],
} as const satisfies Record<WorkspaceBuiltinRole, readonly Permission[]>

export const runnableConnectionPermissions = [
  permission.connExecute,
  permission.connDql,
  permission.connDml,
  permission.connDdl,
] as const satisfies readonly Permission[]

export const allPermissions = Array.from(new Set(Object.values(permission))) as Permission[]

export function hasPermission(permissions: readonly string[] | undefined, required: Permission) {
  return permissions?.includes(required) === true
}

export function hasAnyPermission(permissions: readonly string[] | undefined, required: readonly Permission[]) {
  return required.some((item) => permissions?.includes(item) === true)
}

export function permissionDefinitionMap(definitions: readonly PermissionDefinition[] | undefined) {
  return new Map((definitions ?? []).map((definition) => [definition.key, definition]))
}

export function permissionDisplayName(value: string, definitions: ReadonlyMap<string, PermissionDefinition>) {
  return definitions.get(value)?.label ?? value
}

export function permissionDescription(value: string, definitions: ReadonlyMap<string, PermissionDefinition>) {
  return definitions.get(value)?.description
}

export function permissionGroupName(value: string, definitions: ReadonlyMap<string, PermissionDefinition>) {
  const [prefix] = value.split(':')
  return definitions.get(value)?.group ?? permissionGroupFallback(prefix)
}

function permissionGroupFallback(prefix: string) {
  switch (prefix) {
    case 'org':
      return 'Organization'
    case 'ws':
      return 'Workspace'
    case 'env':
      return 'Environment'
    case 'conn':
      return 'Connection'
    case 'policy':
      return 'Policy'
    default:
      return prefix.toUpperCase()
  }
}
