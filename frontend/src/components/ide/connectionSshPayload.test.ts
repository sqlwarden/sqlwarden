import { describe, expect, it } from 'vitest'

import { emptySshState } from './ConnectionSshFields'
import { sshRevealToState, sshStateToPayload } from './connectionSshPayload'

describe('sshStateToPayload', () => {
  it('maps camelCase state to snake_case payload with numeric port', () => {
    const payload = sshStateToPayload({
      ...emptySshState,
      enabled: true,
      host: 'bastion',
      port: '2222',
      user: 'jump',
      authMethod: 'private_key',
      privateKeyPem: 'KEY',
      passphrase: 'pp',
      fingerprint: 'SHA256:abc',
    })
    expect(payload).toMatchObject({
      enabled: true,
      host: 'bastion',
      port: 2222,
      user: 'jump',
      auth_method: 'private_key',
      private_key_pem: 'KEY',
      passphrase: 'pp',
      fingerprint: 'SHA256:abc',
    })
  })

  it('defaults an empty port to 22', () => {
    expect(sshStateToPayload({ ...emptySshState, enabled: true, port: '' }).port).toBe(22)
  })

  it('carries per-secret clear flags, folding passphrase into the key flag', () => {
    expect(
      sshStateToPayload({ ...emptySshState, clearPassword: true, clearPrivateKey: false }),
    ).toMatchObject({
      clear_password: true,
      clear_private_key: false,
      clear_passphrase: false,
    })
    expect(
      sshStateToPayload({ ...emptySshState, clearPassword: false, clearPrivateKey: true }),
    ).toMatchObject({
      clear_password: false,
      clear_private_key: true,
      clear_passphrase: true,
    })
  })
})

describe('sshRevealToState', () => {
  it('hydrates state, leaves secrets blank, records *_set flags', () => {
    const state = sshRevealToState({
      configured: true,
      enabled: true,
      host: 'bastion',
      port: 22,
      user: 'jump',
      auth_method: 'password',
      known_hosts_entry: 'bastion ssh-ed25519 AAAA',
      fingerprint: '',
      insecure_skip_host_key: false,
      password_set: true,
      private_key_set: false,
    })
    expect(state.password).toBe('')
    expect(state.privateKeyPem).toBe('')
    expect(state.passwordSet).toBe(true)
    expect(state.clearPassword).toBe(false)
    expect(state.clearPrivateKey).toBe(false)
    expect(state.host).toBe('bastion')
    expect(state.port).toBe('22')
  })
})
