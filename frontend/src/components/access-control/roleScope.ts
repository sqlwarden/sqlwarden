import type { RoleScope } from '#/lib/api/types'

export function roleScopeLabel(scope: RoleScope) {
  switch (scope) {
    case 'org':
      return 'Organization'
    case 'workspace':
      return 'Workspace'
    case 'environment':
      return 'Environment'
    case 'connection':
      return 'Connection'
  }
}
