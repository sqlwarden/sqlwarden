import { describe, expect, it } from 'vitest'
import { postgresDriver } from './postgres'
import { mysqlDriver } from './mysql'

describe('postgresDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '5433',
      database: 'analytics',
      username: 'reader',
      password: 'p@ss w/ord',
    }
    const dsn = postgresDriver.buildDSN(fields)
    expect(postgresDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = postgresDriver.buildDSN({
      host: 'localhost',
      port: '5432',
      database: 'app',
      username: 'postgres',
      password: '',
    })
    expect(postgresDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '5432',
      database: 'app',
      username: 'postgres',
      password: '',
    })
  })

  it('does not emit or parse an sslmode query parameter', () => {
    const dsn = postgresDriver.buildDSN({
      host: 'h',
      port: '5432',
      database: 'db',
      username: 'u',
      password: 'p',
    })
    expect(dsn).not.toContain('sslmode')
    expect(
      postgresDriver.parseDSN('postgresql://u:p@h:5432/db?sslmode=verify-full'),
    ).not.toHaveProperty('sslmode')
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(postgresDriver.parseDSN('not-a-url')).toEqual({})
  })
})

describe('mysqlDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '3307',
      database: 'analytics',
      username: 'reader',
      password: 'secret',
    }
    const dsn = mysqlDriver.buildDSN(fields)
    expect(mysqlDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = mysqlDriver.buildDSN({
      host: 'localhost',
      port: '3306',
      database: 'app',
      username: 'root',
      password: '',
    })
    expect(mysqlDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '3306',
      database: 'app',
      username: 'root',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(mysqlDriver.parseDSN('not-a-dsn')).toEqual({})
  })
})
