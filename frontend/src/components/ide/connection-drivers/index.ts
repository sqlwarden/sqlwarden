import { connectableEngines, frontendEngines } from '../engines/registry'
import type { DriverDef } from './types'

export type { DriverDef, FieldLayout } from './types'

export const driverBrands = Object.fromEntries(
  frontendEngines
    .filter((engine) => engine.brand.icon)
    .map((engine) => [
      engine.id,
      { icon: engine.brand.icon!, description: engine.brand.description },
    ]),
)

export const drivers: DriverDef[] = connectableEngines.map((engine) => engine.connection)
export const driverMap = new Map(drivers.map((driver) => [driver.id, driver]))
