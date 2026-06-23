import { describe, it, expect } from 'vitest'
import { columnList, qualifiedColumn } from './clipboard'
import { dialectFor } from '../sqlDialect'

describe('columnList', () => {
  it('joins names with comma+space', () => {
    expect(columnList(['id', 'name', 'email'])).toBe('id, name, email')
  })
  it('returns empty string for no columns', () => {
    expect(columnList([])).toBe('')
  })
})

describe('qualifiedColumn', () => {
  const pg = dialectFor('postgres')
  it('joins bare identifiers with a dot, unquoted', () => {
    expect(qualifiedColumn(pg, 'users', 'id')).toBe('users.id')
  })
  it('quotes identifiers that need quoting', () => {
    expect(qualifiedColumn(pg, 'Order', 'Id')).toBe('"Order"."Id"')
  })
})
