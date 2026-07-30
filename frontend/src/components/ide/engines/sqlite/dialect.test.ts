import { describe, expect, it } from 'vitest'
import { sqliteDialect } from './dialect'

describe('sqlite dialect', () => {
  it('double-quotes identifiers and ignores the main scope', () => {
    expect(sqliteDialect.formatColumn('Mixed')).toBe('"Mixed"')
    expect(sqliteDialect.formatObject([{ kind: 'database', name: 'main' }], 'users')).toBe('users')
  })
})
