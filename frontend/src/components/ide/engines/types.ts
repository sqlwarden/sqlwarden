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
}
