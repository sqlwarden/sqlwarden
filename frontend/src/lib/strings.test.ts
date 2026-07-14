import { describe, expect, it } from 'vitest'
import { slugify } from './strings'

describe('slugify', () => {
  it('normalizes whitespace, casing, and punctuation', () => {
    expect(slugify('  My New Organization!  ')).toBe('my-new-organization')
  })

  it('removes leading and trailing separators', () => {
    expect(slugify('---Example Team---')).toBe('example-team')
  })

  it('applies an optional maximum length', () => {
    expect(slugify('A Very Long Organization Name', { maxLength: 12 })).toBe('a-very-long-')
  })

  it('does not truncate unless requested', () => {
    const value = 'a'.repeat(80)
    expect(slugify(value)).toHaveLength(80)
  })
})
