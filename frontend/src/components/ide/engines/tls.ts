import type { EngineTlsSpec } from './types'

export const standardTlsSpec: EngineTlsSpec = {
  modes: [
    { value: 'disable', label: 'Disable' },
    { value: 'require', label: 'Require (no verification)' },
    { value: 'verify-ca', label: 'Verify CA' },
    { value: 'verify-full', label: 'Verify full' },
  ],
  caBundle: true,
  clientCert: true,
  serverName: true,
}
