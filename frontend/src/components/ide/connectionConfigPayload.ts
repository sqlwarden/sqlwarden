import type {
  ConnectionSecretsPayload,
  ConnectionSshConfig,
  ConnectionTlsConfig,
} from '#/lib/api/types'
import type { SshFormState } from './ConnectionSshFields'
import type { TlsFormState } from './ConnectionTlsFields'
import { sshStateToConfig } from './connectionSshPayload'
import { tlsStateToConfig } from './connectionTlsPayload'

/** A config document is sent when one is already stored (so it can be edited or
 *  disabled) or when the user has turned the feature on. Otherwise it is omitted
 *  so an untouched connection never persists, or compares against, a disabled
 *  placeholder. */
export function sshRequestConfig(
  ssh: SshFormState,
  { supported, stored }: { supported: boolean; stored: boolean },
): { ssh_config: ConnectionSshConfig } | Record<string, never> {
  if (!supported || !(stored || ssh.enabled)) return {}
  return { ssh_config: sshStateToConfig(ssh) }
}

export function tlsRequestConfig(
  tls: TlsFormState,
  { supported, stored }: { supported: boolean; stored: boolean },
): { tls_config: ConnectionTlsConfig } | Record<string, never> {
  if (!supported || !(stored || tls.mode !== 'disable')) return {}
  return { tls_config: tlsStateToConfig(tls) }
}

/** Drops new secret values whose feature is off or whose auth method no longer
 *  uses them. Null clears are retained so a stored secret can still be removed. */
export function applicableSecrets(
  secrets: ConnectionSecretsPayload,
  {
    tlsSupported,
    tls,
    sshSupported,
    ssh,
  }: { tlsSupported: boolean; tls: TlsFormState; sshSupported: boolean; ssh: SshFormState },
): ConnectionSecretsPayload {
  const tlsActive = tlsSupported && tls.mode !== 'disable'
  const sshActive = sshSupported && ssh.enabled
  const result: ConnectionSecretsPayload = { ...secrets }
  if (!tlsActive && result.tls_client_key !== null) delete result.tls_client_key
  if ((!sshActive || ssh.authMethod !== 'password') && result.ssh_password !== null)
    delete result.ssh_password
  if (!sshActive || ssh.authMethod !== 'private_key') {
    if (result.ssh_private_key !== null) delete result.ssh_private_key
    if (result.ssh_passphrase !== null) delete result.ssh_passphrase
  }
  return result
}
