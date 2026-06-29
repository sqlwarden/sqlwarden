import type { ObjectDetail } from '#/lib/api/types'
import type { DriverHooks, ObjectViewModel, SectionDef } from './registry'
import { ColumnsSection } from './sections/ColumnsSection'
import { KeysSection } from './sections/KeysSection'
import { DdlSection } from './sections/DdlSection'
import { ObjectDataPreview } from './ObjectDataPreview'

/** Returns a "source" descriptor's body by title (e.g. "DDL", "Definition"). */
export function sourceDescriptor(detail: ObjectDetail, title: string): string | null {
  const d = (detail.descriptors ?? []).find((x) => x.kind === 'source' && x.title === title)
  return d?.source?.body ?? null
}

export function isRelational(detail: ObjectDetail): boolean {
  return Boolean(detail.relational)
}

/** Common section list shared by every driver. Relational objects (tables,
 *  views) get Columns / Keys & Indexes / DDL / Data; anything else gets a single
 *  generic Overview built from its descriptors. */
export function buildBaseSections(vm: ObjectViewModel, hooks: DriverHooks): SectionDef[] {
  if (!isRelational(vm.detail)) {
    return [{ id: 'overview', label: 'Overview', icon: 'box', render: (m) => <KeysSection vm={m} /> }]
  }
  return [
    { id: 'columns', label: 'Columns', icon: 'table', render: (m) => <ColumnsSection vm={m} extras={hooks.columnExtras?.(m) ?? []} /> },
    { id: 'keys', label: 'Keys & Indexes', icon: 'key-01', render: (m) => <KeysSection vm={m} /> },
    { id: 'ddl', label: 'DDL', icon: 'terminal', render: (m) => <DdlSection vm={m} /> },
    { id: 'data', label: 'Data', icon: 'database', render: (m) => <ObjectDataPreview vm={m} /> },
  ]
}
