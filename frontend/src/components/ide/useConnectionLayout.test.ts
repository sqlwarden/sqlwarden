import { describe, it, expect } from 'vitest'
import { readBooleanPreference } from './useConnectionLayout'

describe('readBooleanPreference', () => {
  it('defaults to true when unset or garbage', () => {
    expect(readBooleanPreference(null)).toBe(true)
    expect(readBooleanPreference('garbage')).toBe(true)
    expect(readBooleanPreference('true')).toBe(true)
  })
  it('is false only for the exact stored value "false"', () => {
    expect(readBooleanPreference('false')).toBe(false)
  })
})
