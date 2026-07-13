import { mysqlEngine } from './mysql'
import { postgresEngine } from './postgres'
import { sqliteEngine } from './sqlite'
import type { FrontendEngine } from './types'

export class UnsupportedFrontendEngineError extends Error {
  constructor(readonly driver: string) {
    super(`Unsupported frontend database driver: ${driver}`)
    this.name = 'UnsupportedFrontendEngineError'
  }
}

export const frontendEngines: readonly FrontendEngine[] = [postgresEngine, mysqlEngine, sqliteEngine]

const engineMap = new Map(frontendEngines.map((engine) => [engine.id, engine]))

export function findFrontendEngine(driver: string): FrontendEngine | undefined {
  return engineMap.get(driver)
}

export function getFrontendEngine(driver: string): FrontendEngine {
  const engine = findFrontendEngine(driver)
  if (!engine) throw new UnsupportedFrontendEngineError(driver)
  return engine
}

export function dialectFor(driver: string) {
  return getFrontendEngine(driver).dialect
}

export const connectableEngines = frontendEngines.filter(
  (engine): engine is FrontendEngine & { connection: NonNullable<FrontendEngine['connection']> } => Boolean(engine.connection),
)
