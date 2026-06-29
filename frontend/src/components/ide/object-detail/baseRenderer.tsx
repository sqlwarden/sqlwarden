import type { ObjectDetail } from '#/lib/api/types'
import type { DriverHooks, ObjectViewModel, SectionDef } from './registry'
import { ColumnsSection } from './sections/ColumnsSection'
import { KeysSection } from './sections/KeysSection'
import { DdlSection } from './sections/DdlSection'
import { ObjectDataPreview } from './ObjectDataPreview'
import { ReadOnlySqlView } from './ReadOnlySqlView'

/** Returns a "source" descriptor's body by title (e.g. "DDL", "Definition"). */
export function sourceDescriptor(detail: ObjectDetail, title: string): string | null {
  const d = (detail.descriptors ?? []).find((x) => x.kind === 'source' && x.title === title)
  return d?.source?.body ?? null
}

export function isRelational(detail: ObjectDetail): boolean {
  return Boolean(detail.relational)
}

/** Common section list shared by every driver. Relational objects (tables,
 *  views) get Columns / Keys & Indexes / DDL / Data; anything else gets a
 *  generic Overview built from its non-source descriptors plus one section per
 *  `source` descriptor (function/procedure/trigger SQL), labeled by its title. */
export function buildBaseSections(vm: ObjectViewModel, hooks: DriverHooks): SectionDef[] {
  if (isRelational(vm.detail)) {
    return [
      { id: 'columns', label: 'Columns', icon: 'table', render: (m) => <ColumnsSection vm={m} extras={hooks.columnExtras?.(m) ?? []} /> },
      { id: 'keys', label: 'Keys & Indexes', icon: 'key-01', render: (m) => <KeysSection vm={m} /> },
      { id: 'ddl', label: 'DDL', icon: 'terminal', render: (m) => <DdlSection vm={m} /> },
      { id: 'data', label: 'Data', icon: 'database', render: (m) => <ObjectDataPreview vm={m} /> },
    ]
  }

  const descriptors = vm.detail.descriptors ?? []
  const sections: SectionDef[] = []
  if (descriptors.some((d) => d.kind !== 'source')) {
    sections.push({ id: 'overview', label: 'Overview', icon: 'box', render: (m) => <KeysSection vm={m} /> })
  }
  descriptors
    .filter((d) => d.kind === 'source')
    .forEach((d, i) => {
      const body = d.source?.body ?? ''
      sections.push({
        id: `source-${i}`,
        label: d.title || 'Source',
        icon: 'terminal',
        render: () => (
          <div className="h-full min-h-0">
            <ReadOnlySqlView value={body} />
          </div>
        ),
      })
    })
  if (sections.length === 0) {
    sections.push({ id: 'overview', label: 'Overview', icon: 'box', render: (m) => <KeysSection vm={m} /> })
  }
  return sections
}
