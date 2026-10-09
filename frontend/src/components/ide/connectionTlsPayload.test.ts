import { describe, expect, it } from 'vitest'

import { emptyTlsState } from './ConnectionTlsFields'
import { tlsConfigToState, tlsStateToConfig } from './connectionTlsPayload'

describe('connectionTlsPayload', () => {
  it('maps form state to config without the client key', () => {
    const config = tlsStateToConfig({
      mode: 'verify-full',
      serverName: 'db.internal',
      caPem: 'CA',
      clientCertPem: 'CERT',
    })
    expect(config).toEqual({
      mode: 'verify-full',
      server_name: 'db.internal',
      ca_pem: 'CA',
      client_cert_pem: 'CERT',
    })
  })

  it('hydrates form state from a stored config', () => {
    expect(tlsConfigToState({ mode: 'require', ca_pem: 'CA' })).toEqual({
      mode: 'require',
      serverName: '',
      caPem: 'CA',
      clientCertPem: '',
    })
    expect(tlsConfigToState(undefined)).toEqual(emptyTlsState)
  })
})
