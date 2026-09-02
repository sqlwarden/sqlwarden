import type { ConnectionTlsReveal } from '#/lib/api/queries/workspace'
import { emptyTlsState, type TlsFormState } from './ConnectionTlsFields'

/** Shape sent to the connection create/update/test endpoints. */
export function tlsStateToPayload(tls: TlsFormState) {
  return {
    mode: tls.mode,
    server_name: tls.serverName,
    ca_pem: tls.caPem,
    client_cert_pem: tls.clientCertPem,
    client_key_pem: tls.clientKeyPem,
  }
}

/** Hydrates edit-form state from the reveal endpoint. The private key is never
 *  returned, so it starts blank; clientKeySet records that one is stored. */
export function tlsRevealToState(r: ConnectionTlsReveal): TlsFormState {
  return {
    ...emptyTlsState,
    mode: r.mode,
    serverName: r.server_name ?? '',
    caPem: r.ca_pem ?? '',
    clientCertPem: r.client_cert_pem ?? '',
    clientKeyPem: '',
    clientKeySet: r.client_key_set,
  }
}
