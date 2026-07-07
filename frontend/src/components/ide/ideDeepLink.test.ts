import { describe, expect, it } from 'vitest'
import type { Connection, Workspace } from '#/lib/api/types'
import { parseIdeSearch, resolveDeepLink } from './ideDeepLink'

function workspace(id: number): Workspace {
  return {
    id,
    owner_type: 'org',
    owner_id: 1,
    name: `ws-${id}`,
    environment_count: 1,
    connection_count: 1,
    created_at: '',
    updated_at: '',
  }
}

function connection(id: number, environmentId: number): Connection {
  return {
    id,
    workspace_id: 42,
    environment_id: environmentId,
    name: `conn-${id}`,
    driver: 'postgres',
    access_mode: 'open',
    created_at: '',
    updated_at: '',
  }
}

describe('parseIdeSearch', () => {
  it('accepts positive integer strings and numbers', () => {
    expect(parseIdeSearch({ ws: '42', conn: 7 })).toEqual({ ws: 42, conn: 7 })
  })

  it('drops invalid values', () => {
    expect(parseIdeSearch({ ws: 'abc', conn: -1 })).toEqual({})
    expect(parseIdeSearch({ ws: 1.5, conn: null })).toEqual({})
    expect(parseIdeSearch({})).toEqual({})
  })
})

describe('resolveDeepLink', () => {
  const workspaces = [workspace(42), workspace(43)]

  it('activates a matching workspace and is ready without conn', () => {
    expect(resolveDeepLink({ ws: 42 }, workspaces, undefined)).toEqual({
      activateWorkspaceId: 42,
      expandKeys: [],
      ready: true,
    })
  })

  it('ignores a ws that is not accessible', () => {
    expect(resolveDeepLink({ ws: 99, conn: 7 }, workspaces, undefined)).toEqual({
      activateWorkspaceId: undefined,
      expandKeys: [],
      ready: true,
    })
  })

  it('waits for connections when conn is present', () => {
    expect(resolveDeepLink({ ws: 42, conn: 7 }, workspaces, undefined).ready).toBe(false)
  })

  it('expands the environment and connection nodes once connections load', () => {
    const result = resolveDeepLink({ ws: 42, conn: 7 }, workspaces, [connection(7, 3)])
    expect(result).toEqual({
      activateWorkspaceId: 42,
      expandKeys: ['env:3', 'conn:7'],
      ready: true,
    })
  })

  it('is ready with no expansion when the connection does not exist', () => {
    const result = resolveDeepLink({ ws: 42, conn: 7 }, workspaces, [connection(8, 3)])
    expect(result).toEqual({ activateWorkspaceId: 42, expandKeys: [], ready: true })
  })
})
