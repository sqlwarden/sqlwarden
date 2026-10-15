import { describe, expect, it } from 'vitest'
import { buildSetupPayload } from './setup-payload'

describe('buildSetupPayload', () => {
  const values = {
    name: 'Ada',
    email: 'ada@example.com',
    password: 'longpassword',
    organization_name: 'Acme',
    organization_slug: 'acme',
  }

  it('sends the form when setup requires input', () => {
    expect(buildSetupPayload(values, true)).toEqual(values)
  })

  it('sends an empty body when setup requires no input', () => {
    expect(buildSetupPayload(values, false)).toEqual({})
  })
})
