import { describe, it, expect } from 'vitest'
import { readBooleanPreference, readLayoutPreference } from './useConnectionLayout'

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

describe('readLayoutPreference', () => {
  it('returns undefined when unset', () => {
    expect(readLayoutPreference(null)).toBeUndefined()
  })
  it('parses a valid layout map', () => {
    expect(readLayoutPreference('{"explorer-connections":60,"explorer-schema":40}')).toEqual({
      'explorer-connections': 60,
      'explorer-schema': 40,
    })
  })
  it('falls back to undefined for malformed JSON', () => {
    expect(readLayoutPreference('not json')).toBeUndefined()
  })
  it('falls back to undefined for a non-object or non-numeric-valued shape', () => {
    expect(readLayoutPreference('[1,2,3]')).toBeUndefined()
    expect(readLayoutPreference('"a string"')).toBeUndefined()
    expect(readLayoutPreference('{"explorer-connections":"60%"}')).toBeUndefined()
  })
})
