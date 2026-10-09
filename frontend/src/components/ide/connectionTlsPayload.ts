import type { ConnectionTlsConfig } from '#/lib/api/types'
import { emptyTlsState, type TlsFormState } from './ConnectionTlsFields'

export function tlsStateToConfig(tls: TlsFormState): ConnectionTlsConfig {
  return {
    mode: tls.mode,
    server_name: tls.serverName,
    ca_pem: tls.caPem,
    client_cert_pem: tls.clientCertPem,
  }
}

export function tlsConfigToState(config: ConnectionTlsConfig | undefined): TlsFormState {
  if (!config) return emptyTlsState
  return {
    mode: config.mode,
    serverName: config.server_name ?? '',
    caPem: config.ca_pem ?? '',
    clientCertPem: config.client_cert_pem ?? '',
  }
}
