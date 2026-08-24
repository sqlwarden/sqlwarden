import type { DriverDef } from '../connection-drivers/types'
import type { SqlDialect } from '../dialect'
import type { ObjectDetailHooks } from '../object-detail/types'
import type { DiagramHooks } from '../schema-diagram/types'

export interface EngineBrand {
  icon?: string
  description: string
}

export interface FrontendEngine {
  id: string
  label: string
  brand: EngineBrand
  dialect: SqlDialect
  objectDetail: ObjectDetailHooks
  diagram: DiagramHooks
  connection?: DriverDef
  /** Set when this engine can silently end a manual transaction — e.g. an
   *  implicit commit on DDL. Shown as a persistent warning in manual mode.
   *  Leave unset for engines with fully transactional DDL. */
  manualTransactionWarning?: string
}
