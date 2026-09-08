import { describe, expect, it } from 'vitest'
import { sqlServerDialect } from './dialect'

describe('sqlserver dialect', () => {
  it('bracket-quotes identifiers', () => {
    expect(sqlServerDialect.formatColumn('email')).toBe('email')
    expect(sqlServerDialect.formatColumn('UserId')).toBe('[UserId]')
    expect(sqlServerDialect.formatColumn('a]b')).toBe('[a]]b]')
  })

  it('omits the schema qualifier for the default dbo schema', () => {
    expect(
      sqlServerDialect.formatObject(
        [
          { kind: 'database', name: 'appdb' },
          { kind: 'schema', name: 'dbo' },
        ],
        'users',
      ),
    ).toBe('users')
  })

  it('schema-qualifies objects outside dbo', () => {
    expect(
      sqlServerDialect.formatObject(
        [
          { kind: 'database', name: 'appdb' },
          { kind: 'schema', name: 'Sales' },
        ],
        'Orders',
      ),
    ).toBe('[Sales].[Orders]')
  })

  it('builds bounded count queries using TOP instead of LIMIT, with an aliased projection', () => {
    const query = sqlServerDialect.boundedCountQuery(
      {
        scope: [
          { kind: 'database', name: 'appdb' },
          { kind: 'schema', name: 'dbo' },
        ],
        kind: 'table',
        name: 'Orders',
      },
      5,
    )
    expect(query).toBe(
      'SELECT COUNT(*) FROM (SELECT TOP (5) 1 AS n FROM [Orders]) AS _warden_count',
    )
    expect(query).not.toContain('LIMIT')
  })
})
