import { describe, expect, it } from 'vitest'
import { sqliteDialect } from './dialect'

describe('sqlite dialect', () => {
  it('double-quotes identifiers only when required', () => {
    expect(sqliteDialect.formatColumn('email')).toBe('email')
    expect(sqliteDialect.formatColumn('Mixed')).toBe('"Mixed"')
    expect(sqliteDialect.formatColumn('my col')).toBe('"my col"')
    expect(sqliteDialect.formatColumn('a"b')).toBe('"a""b"')
  })

  it('leaves objects in the main database unqualified', () => {
    expect(sqliteDialect.formatObject([{ kind: 'database', name: 'main' }], 'users')).toBe('users')
    expect(sqliteDialect.formatObject([], 'users')).toBe('users')
  })

  it('qualifies objects in an attached database', () => {
    expect(sqliteDialect.formatObject([{ kind: 'database', name: 'audit' }], 'events')).toBe(
      'audit.events',
    )
    expect(sqliteDialect.formatObject([{ kind: 'database', name: 'Reporting' }], 'Daily')).toBe(
      '"Reporting"."Daily"',
    )
  })

  it('builds preview and count queries with dialect formatting', () => {
    const ref = { scope: [{ kind: 'database', name: 'main' }], kind: 'table', name: 'users' }
    expect(sqliteDialect.previewQuery(ref)).toBe('SELECT * FROM users')
    expect(sqliteDialect.exactCountQuery(ref)).toBe('SELECT COUNT(*) FROM users')
    expect(sqliteDialect.boundedCountQuery(ref, 10001)).toBe(
      'SELECT COUNT(*) FROM (SELECT 1 FROM users LIMIT 10001) AS _warden_count',
    )
    expect(
      sqliteDialect.previewQuery({
        ...ref,
        scope: [{ kind: 'database', name: 'audit' }],
        name: 'Daily',
      }),
    ).toBe('SELECT * FROM audit."Daily"')
  })
})
