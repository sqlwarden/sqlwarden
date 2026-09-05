import type { DriverDef } from '../connection-drivers/types'
import type { SqlDialect } from '../dialect'
import type { ObjectDetailHooks } from '../object-detail/types'
import type { DiagramHooks } from '../schema-diagram/types'

export interface EngineBrand {
  icon?: string
  description: string
}

export type TlsMode = 'disable' | 'require' | 'verify-ca' | 'verify-full'

export interface EngineTlsSpec {
  modes: { value: TlsMode; label: string }[]
  caBundle: boolean
  clientCert: boolean
  serverName: boolean
}

export interface FrontendEngine {
  id: string
  label: string
  brand: EngineBrand
  dialect: SqlDialect
  objectDetail: ObjectDetailHooks
  diagram: DiagramHooks
  connection?: DriverDef
  /** Structured TLS configuration this engine accepts (CA bundle, client cert,
   *  verification mode, server name). Unset for engines with no network TLS
   *  surface, e.g. SQLite. */
  tls?: EngineTlsSpec
  /** Set when this engine can route its transport through an SSH bastion.
   *  Uniform across network engines; SQLite leaves it unset. */
  sshTunnel?: boolean
  /** Set when this engine can silently end a manual transaction — e.g. an
   *  implicit commit on DDL. Shown as a persistent warning in manual mode.
   *  Leave unset for engines with fully transactional DDL. */
  manualTransactionWarning?: string
  /** Set when this engine has a backend semantic-completion capability
   *  (`sql.complete`): the IDE calls `/completion` for schema-aware suggestions
   *  instead of falling back to lexical vocabulary only. */
  semanticCompletion?: boolean
}
