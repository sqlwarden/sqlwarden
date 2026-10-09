import { describe, expect, it } from 'vitest'
import { emptySshState, type SshFormState } from './ConnectionSshFields'
import { emptyTlsState, type TlsFormState } from './ConnectionTlsFields'
import { applicableSecrets, sshRequestConfig, tlsRequestConfig } from './connectionConfigPayload'

const enabledSsh: SshFormState = { ...emptySshState, enabled: true, host: 'bastion' }
const verifyTls: TlsFormState = { ...emptyTlsState, mode: 'verify-full' }

describe('sshRequestConfig', () => {
  it('omits a disabled tunnel when nothing is stored', () => {
    expect(sshRequestConfig(emptySshState, { supported: true, stored: false })).toEqual({})
  })

  it('sends a disabled tunnel when a config is stored so it can be turned off', () => {
    const result = sshRequestConfig(emptySshState, { supported: true, stored: true })
    expect(result).toEqual({ ssh_config: expect.objectContaining({ enabled: false }) })
  })

  it('sends an enabled tunnel', () => {
    const result = sshRequestConfig(enabledSsh, { supported: true, stored: false })
    expect(result).toEqual({ ssh_config: expect.objectContaining({ enabled: true }) })
  })

  it('never sends for an engine without tunnel support', () => {
    expect(sshRequestConfig(enabledSsh, { supported: false, stored: true })).toEqual({})
  })
})

describe('tlsRequestConfig', () => {
  it('omits mode disable when nothing is stored', () => {
    expect(tlsRequestConfig(emptyTlsState, { supported: true, stored: false })).toEqual({})
  })

  it('sends mode disable when a config is stored', () => {
    expect(tlsRequestConfig(emptyTlsState, { supported: true, stored: true })).toEqual({
      tls_config: expect.objectContaining({ mode: 'disable' }),
    })
  })

  it('sends an active mode and never sends for an engine without TLS', () => {
    expect(tlsRequestConfig(verifyTls, { supported: true, stored: false })).toEqual({
      tls_config: expect.objectContaining({ mode: 'verify-full' }),
    })
    expect(tlsRequestConfig(verifyTls, { supported: false, stored: true })).toEqual({})
  })
})

describe('applicableSecrets', () => {
  const all = {
    password: 'db',
    ssh_password: 'sp',
    ssh_private_key: 'key',
    ssh_passphrase: 'phrase',
    tls_client_key: 'tk',
  }

  it('keeps the database password and drops every dependent secret when features are off', () => {
    expect(
      applicableSecrets(all, {
        tlsSupported: true,
        tls: emptyTlsState,
        sshSupported: true,
        ssh: emptySshState,
      }),
    ).toEqual({ password: 'db' })
  })

  it('preserves stored-secret clears when features are off', () => {
    expect(
      applicableSecrets(
        { ssh_password: null, ssh_private_key: null, ssh_passphrase: null, tls_client_key: null },
        {
          tlsSupported: true,
          tls: emptyTlsState,
          sshSupported: true,
          ssh: emptySshState,
        },
      ),
    ).toEqual({
      ssh_password: null,
      ssh_private_key: null,
      ssh_passphrase: null,
      tls_client_key: null,
    })
  })

  it('preserves a password clear when switching to private key auth', () => {
    expect(
      applicableSecrets(
        { ssh_password: null, ssh_private_key: 'replacement' },
        {
          tlsSupported: false,
          tls: emptyTlsState,
          sshSupported: true,
          ssh: { ...enabledSsh, authMethod: 'private_key' },
        },
      ),
    ).toEqual({ ssh_password: null, ssh_private_key: 'replacement' })
  })

  it('keeps ssh_password only for password auth', () => {
    const result = applicableSecrets(all, {
      tlsSupported: true,
      tls: verifyTls,
      sshSupported: true,
      ssh: enabledSsh,
    })
    expect(result).toEqual({ password: 'db', ssh_password: 'sp', tls_client_key: 'tk' })
  })

  it('keeps the key and passphrase only for private key auth, and preserves null clears', () => {
    const result = applicableSecrets(
      { ...all, ssh_private_key: null },
      {
        tlsSupported: false,
        tls: verifyTls,
        sshSupported: true,
        ssh: { ...enabledSsh, authMethod: 'private_key' },
      },
    )
    expect(result).toEqual({ password: 'db', ssh_private_key: null, ssh_passphrase: 'phrase' })
  })
})
