import { describe, expect, it } from 'vitest'

import { emptySshState } from './ConnectionSshFields'
import { sshConfigToState, sshStateToConfig } from './connectionSshPayload'

describe('sshStateToConfig', () => {
  it('maps camelCase state to snake_case config with a numeric port', () => {
    expect(
      sshStateToConfig({
        ...emptySshState,
        enabled: true,
        host: 'bastion',
        port: '2222',
        user: 'jump',
        authMethod: 'private_key',
        fingerprint: 'SHA256:abc',
      }),
    ).toMatchObject({
      enabled: true,
      host: 'bastion',
      port: 2222,
      user: 'jump',
      auth_method: 'private_key',
      fingerprint: 'SHA256:abc',
    })
  })

  it('defaults an empty port to 22', () => {
    expect(sshStateToConfig({ ...emptySshState, enabled: true, port: '' }).port).toBe(22)
  })

  it('never carries secret material', () => {
    expect(Object.keys(sshStateToConfig({ ...emptySshState, enabled: true }))).not.toContain(
      'password',
    )
  })
})

describe('sshConfigToState', () => {
  it('returns the empty state without a stored config', () => {
    expect(sshConfigToState(undefined)).toEqual(emptySshState)
  })

  it('hydrates form state from a stored config', () => {
    expect(
      sshConfigToState({
        enabled: true,
        host: 'bastion',
        port: 2222,
        user: 'jump',
        auth_method: 'private_key',
        known_hosts_entry: 'bastion ssh-ed25519 AAAA',
        insecure_skip_host_key: false,
      }),
    ).toEqual({
      enabled: true,
      host: 'bastion',
      port: '2222',
      user: 'jump',
      authMethod: 'private_key',
      knownHostsEntry: 'bastion ssh-ed25519 AAAA',
      fingerprint: '',
      insecureSkipHostKey: false,
    })
  })
})
