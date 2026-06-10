// Stable permission names used by UI capability checks. Permission catalogs,
// scope maps, resource maps, labels, and descriptions come from the backend API.
import type { PermissionDefinition } from '#/lib/api/types'

export const permission = {
  orgRead: 'org:read',
  orgWrite: 'org:write',
  orgDelete: 'org:delete',
  orgInvite: 'org:invite',
  orgTransferOwnership: 'org:transfer_ownership',

  wsRead: 'ws:read',
  wsWrite: 'ws:write',
  wsCreate: 'ws:create',
  wsDelete: 'ws:delete',
  wsFileRead: 'wsfile:read',
  wsFileCreate: 'wsfile:create',
  wsFileWrite: 'wsfile:write',
  wsFileDelete: 'wsfile:delete',

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

export type Permission = string
export type PermissionScope = 'org' | 'workspace' | 'environment' | 'connection'

export const builtinRole = {
  organizationOwner: 'Owner',
  organizationAdmin: 'Administrator',
  organizationMember: 'Baseline Access',
  workspaceAdmin: 'Workspace Admin',
  workspaceMember: 'Workspace Member',
} as const

export type BuiltinRole = (typeof builtinRole)[keyof typeof builtinRole]
export type OrgBuiltinRole =
  | typeof builtinRole.organizationOwner
  | typeof builtinRole.organizationAdmin
  | typeof builtinRole.organizationMember
export type WorkspaceBuiltinRole = typeof builtinRole.workspaceAdmin | typeof builtinRole.workspaceMember

export const runnableConnectionPermissions = [
  permission.connExecute,
  permission.connDql,
  permission.connDml,
  permission.connDdl,
] as const satisfies readonly Permission[]

export const protectedOrgPolicyPermissions = [
  permission.orgDelete,
  permission.orgTransferOwnership,
] as const satisfies readonly Permission[]

export function hasPermission(permissions: readonly string[] | undefined, required: Permission) {
  return permissions?.includes(required) === true
}

export function hasAnyPermission(permissions: readonly string[] | undefined, required: readonly Permission[]) {
  return required.some((item) => permissions?.includes(item) === true)
}

export function protectedOrgPolicyMessage(rolePermissions: readonly string[] | undefined, effectivePermissions: readonly string[] | undefined) {
  const missingProtectedPermission = protectedOrgPolicyPermissions.some(
    (item) => rolePermissions?.includes(item) === true && effectivePermissions?.includes(item) !== true,
  )

  if (!missingProtectedPermission) {
    return undefined
  }

  return 'Only users who already have organization deletion or ownership transfer permission can manage policies that grant those permissions.'
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
    case 'wsfile':
      return 'Workspace Files'
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
