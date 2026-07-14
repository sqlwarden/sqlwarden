import { getFrontendEngine } from '../engines/registry'
import type { DiagramHooks } from './types'

export type { DiagramHooks } from './types'

export function getDiagramHooks(driver: string): DiagramHooks {
  return getFrontendEngine(driver).diagram
}
