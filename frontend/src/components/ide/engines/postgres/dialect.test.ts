import { describe, expect, it } from 'vitest'
import { postgresDialect } from './dialect'

describe('postgres dialect', () => {
  it('quotes identifiers only when required', () => {
    expect(postgresDialect.formatColumn('email')).toBe('email')
    expect(postgresDialect.formatColumn('UserId')).toBe('"UserId"')
    expect(postgresDialect.formatColumn('my col')).toBe('"my col"')
    expect(postgresDialect.formatColumn('a"b')).toBe('"a""b"')
  })

  it('leaves objects in the public schema unqualified', () => {
    expect(postgresDialect.formatObject('public', 'users')).toBe('users')
  })

  it('qualifies objects outside the public schema', () => {
    expect(postgresDialect.formatObject('analytics', 'events')).toBe('analytics.events')
    expect(postgresDialect.formatObject('Reporting', 'Daily')).toBe('"Reporting"."Daily"')
  })

  it('builds preview and count queries with dialect formatting', () => {
    const ref = { namespace: 'public', kind: 'table', name: 'users' }
    expect(postgresDialect.previewQuery(ref)).toBe('SELECT * FROM users')
    expect(postgresDialect.exactCountQuery(ref)).toBe('SELECT COUNT(*) FROM users')
    expect(postgresDialect.boundedCountQuery(ref, 10001)).toBe(
      'SELECT COUNT(*) FROM (SELECT 1 FROM users LIMIT 10001) AS _warden_count',
    )
    expect(postgresDialect.previewQuery({ ...ref, namespace: 'analytics', name: 'Daily' })).toBe(
      'SELECT * FROM analytics."Daily"',
    )
  })
})
