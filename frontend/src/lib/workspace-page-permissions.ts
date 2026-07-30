import { permission, type Permission } from '#/lib/permissions'

/** Permission sets that gate each workspace admin page. Shared by the
 *  workspace sidebar (nav item visibility + path guard) and the workspace
 *  Overview hub cards so the two can never disagree. */
export const workspaceEnvironmentPagePermissions = [
  permission.envRead,
  permission.envWrite,
  permission.envCreate,
  permission.envDelete,
  permission.envDeploy,
  permission.connRead,
  permission.connUpdate,
  permission.connCreate,
  permission.connDelete,
  permission.connExecute,
  permission.connDql,
  permission.connDml,
  permission.connDdl,
] as const satisfies readonly Permission[]

export const workspaceConnectionPagePermissions = [
  permission.connRead,
  permission.connUpdate,
  permission.connCreate,
  permission.connDelete,
  permission.connExecute,
  permission.connDql,
  permission.connDml,
  permission.connDdl,
] as const satisfies readonly Permission[]

export const workspacePolicyPagePermissions = [
  permission.policyRead,
  permission.policyModify,
] as const satisfies readonly Permission[]

export const workspaceSettingsPagePermissions = [
  permission.wsRead,
  permission.wsWrite,
] as const satisfies readonly Permission[]
