import { describe, expect, it } from 'vitest'
import { queryKeys } from './query-keys'

describe('queryKeys', () => {
  it('uses collection scopes as prefixes for filtered lists', () => {
    const scope = queryKeys.orgWorkspaceMembersScope('acme', 7)
    const list = queryKeys.orgWorkspaceMembers('acme', 7, { page: 2, q: 'sam' })

    expect(list.slice(0, scope.length)).toEqual(scope)
    expect(list.at(-1)).toEqual({ page: 2, q: 'sam' })
  })

  it('keeps file content addressing consistent across load and eviction', () => {
    expect(queryKeys.fileContent('acme', 7, 42)).toEqual([
      'file-content',
      'acme',
      7,
      42,
    ])
  })

  it('separates workspace-scoped collections', () => {
    expect(queryKeys.orgWorkspaceConnectionsScope('acme', 7)).not.toEqual(
      queryKeys.orgWorkspaceConnectionsScope('acme', 8),
    )
    expect(queryKeys.orgWorkspacePoliciesScope('acme', 7)).not.toEqual(
      queryKeys.orgWorkspaceRolesScope('acme', 7),
    )
  })
})
