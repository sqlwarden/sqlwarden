import { describe, expect, it } from 'vitest'
import { tidbDialect } from './dialect'

describe('tidb dialect', () => {
  it('quotes a completion name only when it is not a bare identifier', () => {
    expect(tidbDialect.formatIdentifier('Orders')).toBe('Orders')
    expect(tidbDialect.formatIdentifier('order details')).toBe('`order details`')
  })
})
