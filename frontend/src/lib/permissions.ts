// Mirrors internal/access/permissions.go. Keep names and groupings aligned with
// the backend permission model so UI capability checks use backend vocabulary.
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
  owner: scopePermissions.org,
  admin: [
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
} as const satisfies Record<'owner' | 'admin', readonly Permission[]>

export const workspaceBuiltinRoles = {
  'ws:admin': [
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
  'ws:member': [
    permission.wsRead,
    permission.envRead,
    permission.connRead,
    permission.connDql,
  ],
} as const satisfies Record<'ws:admin' | 'ws:member', readonly Permission[]>

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
