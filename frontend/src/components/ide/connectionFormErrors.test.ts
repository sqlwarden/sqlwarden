import { describe, expect, it } from 'vitest'
import { mapConnectionFieldErrors } from './connectionFormErrors'

describe('mapConnectionFieldErrors', () => {
  it('routes params errors to their inputs and the rest to the form', () => {
    expect(
      mapConnectionFieldErrors({
        name: 'Name is required.',
        environment_id: 'Environment is required.',
        'params.host': 'Host is required.',
        'secrets.ssh_password': 'SSH password is required for password authentication.',
      }),
    ).toEqual({
      name: 'Name is required.',
      environmentId: 'Environment is required.',
      fields: { host: 'Host is required.' },
      form: 'SSH password is required for password authentication.',
    })
  })

  it('keeps the first unmatched message', () => {
    expect(
      mapConnectionFieldErrors({
        'tls_config.mode': 'Unsupported TLS verification mode.',
        driver: 'x',
      }).form,
    ).toBe('Unsupported TLS verification mode.')
  })
})
