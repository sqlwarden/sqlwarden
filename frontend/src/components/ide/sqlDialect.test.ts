import { describe, it, expect } from 'vitest'
import { dialectFor } from './sqlDialect'

describe('postgres dialect', () => {
  const d = dialectFor('postgres')
  it('inserts bare lowercase names; quotes shape that requires it', () => {
    expect(d.formatColumn('email')).toBe('email')
    expect(d.formatColumn('UserId')).toBe('"UserId"')
    expect(d.formatColumn('my col')).toBe('"my col"')
    expect(d.formatColumn('a"b')).toBe('"a""b"')
  })
  it('leaves objects in the public schema bare', () => {
    expect(d.formatObject('public', 'users')).toBe('users')
  })
  it('qualifies objects outside public with the schema', () => {
    expect(d.formatObject('analytics', 'events')).toBe('analytics.events')
    expect(d.formatObject('Reporting', 'Daily')).toBe('"Reporting"."Daily"')
  })
})

describe('mysql dialect', () => {
  const d = dialectFor('mysql')
  it('backtick-quotes and never qualifies (single database namespace)', () => {
    expect(d.formatColumn('email')).toBe('email')
    expect(d.formatColumn('UserId')).toBe('`UserId`')
    expect(d.formatColumn('a`b')).toBe('`a``b`')
    expect(d.formatObject('appdb', 'users')).toBe('users')
  })
})

describe('sqlite dialect', () => {
  const d = dialectFor('sqlite')
  it('double-quotes and never qualifies (single namespace)', () => {
    expect(d.formatColumn('Mixed')).toBe('"Mixed"')
    expect(d.formatObject('main', 'users')).toBe('users')
  })
})

describe('unknown driver', () => {
  it('falls back to the postgres dialect', () => {
    const d = dialectFor('cockroach')
    expect(d.formatColumn('Mixed')).toBe('"Mixed"')
    expect(d.formatObject('analytics', 'events')).toBe('analytics.events')
  })
})
