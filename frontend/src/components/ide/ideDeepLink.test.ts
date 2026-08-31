import { describe, expect, it } from 'vitest'
import type { Connection } from '#/lib/api/types'
import { parseIdeSearch, resolveDeepLink } from './ideDeepLink'

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
    expect(parseIdeSearch({ conn: '7' })).toEqual({ conn: 7 })
    expect(parseIdeSearch({ conn: 7 })).toEqual({ conn: 7 })
  })

  it('drops invalid values', () => {
    expect(parseIdeSearch({ conn: 'abc' })).toEqual({})
    expect(parseIdeSearch({ conn: -1 })).toEqual({})
    expect(parseIdeSearch({ conn: null })).toEqual({})
    expect(parseIdeSearch({})).toEqual({})
  })
})

describe('resolveDeepLink', () => {
  it('is ready with no expansion when there is no conn', () => {
    expect(resolveDeepLink({}, undefined)).toEqual({ expandKeys: [], ready: true })
  })

  it('waits for connections when conn is present', () => {
    expect(resolveDeepLink({ conn: 7 }, undefined).ready).toBe(false)
  })

  it('expands the environment and connection nodes once connections load', () => {
    const result = resolveDeepLink({ conn: 7 }, [connection(7, 3)])
    expect(result).toEqual({ expandKeys: ['env:3', 'conn:7'], ready: true })
  })

  it('is ready with no expansion when the connection does not exist', () => {
    const result = resolveDeepLink({ conn: 7 }, [connection(8, 3)])
    expect(result).toEqual({ expandKeys: [], ready: true })
  })
})
