import { describe, expect, it } from 'vitest'
import type { RoleScope } from '#/lib/api/types'
import { roleScopeLabel } from './roleScope'

describe('roleScopeLabel', () => {
  it.each<[RoleScope, string]>([
    ['org', 'Organization'],
    ['workspace', 'Workspace'],
    ['environment', 'Environment'],
    ['connection', 'Connection'],
  ])('labels the %s scope', (scope, expected) => {
    expect(roleScopeLabel(scope)).toBe(expected)
  })
})
