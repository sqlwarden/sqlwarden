import { emptySshState, type SshAuthMethod, type SshFormState } from './ConnectionSshFields'

export interface ConnectionSshReveal {
  configured: boolean
  enabled: boolean
  host: string
  port: number
  user: string
  auth_method: string
  known_hosts_entry: string
  fingerprint: string
  insecure_skip_host_key: boolean
  password_set: boolean
  private_key_set: boolean
}

export function sshStateToPayload(state: SshFormState) {
  const port = Number.parseInt(state.port, 10)
  return {
    enabled: state.enabled,
    host: state.host,
    port: Number.isFinite(port) && port > 0 ? port : 22,
    user: state.user,
    auth_method: state.authMethod,
    password: state.password,
    private_key_pem: state.privateKeyPem,
    passphrase: state.passphrase,
    known_hosts_entry: state.knownHostsEntry,
    fingerprint: state.fingerprint,
    insecure_skip_host_key: state.insecureSkipHostKey,
    clear_password: state.clearPassword,
    clear_private_key: state.clearPrivateKey,
    clear_passphrase: state.clearPrivateKey,
  }
}

export function sshRevealToState(r: ConnectionSshReveal): SshFormState {
  return {
    ...emptySshState,
    enabled: r.enabled,
    host: r.host ?? '',
    port: r.port ? String(r.port) : '22',
    user: r.user ?? '',
    authMethod: (r.auth_method as SshAuthMethod) || 'password',
    password: '',
    privateKeyPem: '',
    passphrase: '',
    knownHostsEntry: r.known_hosts_entry ?? '',
    fingerprint: r.fingerprint ?? '',
    insecureSkipHostKey: r.insecure_skip_host_key,
    passwordSet: r.password_set,
    privateKeySet: r.private_key_set,
    clearPassword: false,
    clearPrivateKey: false,
  }
}
