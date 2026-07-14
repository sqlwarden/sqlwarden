import { getFrontendEngine } from '../engines/registry'
import { buildBaseSections } from './baseRenderer'
import type { ObjectRenderer } from './types'

export type {
  ColumnExtra,
  HeaderBadge,
  ObjectDetailHooks as DriverHooks,
  ObjectRenderer,
  ObjectViewModel,
  SectionDef,
} from './types'

export function getObjectRenderer(driver: string): ObjectRenderer {
  const hooks = getFrontendEngine(driver).objectDetail
  return {
    sections: (viewModel) => buildBaseSections(viewModel, hooks),
    headerBadges: (viewModel) => hooks.headerBadges?.(viewModel) ?? [],
    columnExtras: (viewModel) => hooks.columnExtras?.(viewModel) ?? [],
  }
}
