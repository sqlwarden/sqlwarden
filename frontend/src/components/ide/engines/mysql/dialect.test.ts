import { describe, expect, it } from 'vitest'
import { mysqlDialect } from './dialect'

describe('mysql dialect', () => {
  it('backtick-quotes identifiers and ignores the database scope', () => {
    expect(mysqlDialect.formatColumn('email')).toBe('email')
    expect(mysqlDialect.formatColumn('UserId')).toBe('`UserId`')
    expect(mysqlDialect.formatColumn('a`b')).toBe('`a``b`')
    expect(mysqlDialect.formatObject([{ kind: 'database', name: 'appdb' }], 'users')).toBe('users')
  })

  it('builds bounded count queries with dialect formatting', () => {
    expect(
      mysqlDialect.boundedCountQuery(
        {
          scope: [{ kind: 'database', name: 'appdb' }],
          kind: 'table',
          name: 'Orders',
        },
        5,
      ),
    ).toBe('SELECT COUNT(*) FROM (SELECT 1 FROM `Orders` LIMIT 5) AS _warden_count')
  })
})
