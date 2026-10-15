import type { ConnectionSshConfig } from '#/lib/api/types'
import { emptySshState, type SshFormState } from './ConnectionSshFields'

export function sshStateToConfig(state: SshFormState): ConnectionSshConfig {
  const port = Number.parseInt(state.port, 10)
  return {
    enabled: state.enabled,
    host: state.host,
    port: Number.isFinite(port) && port > 0 ? port : 22,
    user: state.user,
    auth_method: state.authMethod,
    known_hosts_entry: state.knownHostsEntry,
    fingerprint: state.fingerprint,
    insecure_skip_host_key: state.insecureSkipHostKey,
  }
}

export function sshConfigToState(config: ConnectionSshConfig | undefined): SshFormState {
  if (!config) return emptySshState
  return {
    enabled: config.enabled,
    host: config.host ?? '',
    port: config.port ? String(config.port) : '22',
    user: config.user ?? '',
    authMethod: config.auth_method ?? 'password',
    knownHostsEntry: config.known_hosts_entry ?? '',
    fingerprint: config.fingerprint ?? '',
    insecureSkipHostKey: config.insecure_skip_host_key ?? false,
  }
}
