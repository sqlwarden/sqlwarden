import { describe, expect, it } from 'vitest'
import { mariadbDialect } from './dialect'

describe('mariadb dialect', () => {
  it('backtick-quotes identifiers and ignores the database scope', () => {
    expect(mariadbDialect.formatColumn('email')).toBe('email')
    expect(mariadbDialect.formatColumn('UserId')).toBe('`UserId`')
    expect(mariadbDialect.formatColumn('a`b')).toBe('`a``b`')
    expect(mariadbDialect.formatObject([{ kind: 'database', name: 'appdb' }], 'users')).toBe(
      'users',
    )
  })

  it('builds bounded count queries with dialect formatting', () => {
    expect(
      mariadbDialect.boundedCountQuery(
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
