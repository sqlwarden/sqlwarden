import type { ObjectDetail } from '#/lib/api/types'
import type { DriverHooks, ObjectViewModel, SectionDef } from './registry'
import { ColumnsSection } from './sections/ColumnsSection'
import { DescriptorList, KeysSection } from './sections/KeysSection'
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

/** Whether the object's kind carries a reconstructable DDL/definition. Driven by
 *  the engine's SchemaSpec so kinds an engine cannot define (e.g. Postgres
 *  type/domain) do not get an empty DDL tab. Defaults to true when the spec or
 *  the kind entry is unavailable, preserving the pre-flag behavior. */
function kindHasDefinition(vm: ObjectViewModel): boolean {
  const kinds = vm.spec?.kinds
  if (!kinds) return true
  const entry = kinds.find((k) => k.kind === vm.detail.ref.kind)
  return entry ? entry.has_definition === true : true
}

const ddlSection: SectionDef = {
  id: 'ddl',
  label: 'DDL',
  icon: 'terminal',
  render: (m) => <DdlSection vm={m} />,
}

/** Common section list shared by every driver. Relational objects (tables,
 *  views) get Columns / Keys & Indexes / DDL / Data; anything else gets a
 *  generic Overview built from its non-source descriptors plus one section per
 *  inline `source` descriptor (function/procedure/trigger SQL), labeled by its
 *  title. When no `source` descriptor is inlined, a DDL section fetches the
 *  definition on demand so procedures, triggers, sequences, and materialized
 *  views still show their canonical text — but only for kinds the engine can
 *  actually define (see kindHasDefinition). */
export function buildBaseSections(vm: ObjectViewModel, hooks: DriverHooks): SectionDef[] {
  const showDdl = kindHasDefinition(vm)

  if (isRelational(vm.detail)) {
    const sections: SectionDef[] = [
      {
        id: 'columns',
        label: 'Columns',
        icon: 'table',
        render: (m) => <ColumnsSection vm={m} extras={hooks.columnExtras?.(m) ?? []} />,
      },
      {
        id: 'keys',
        label: 'Keys & Indexes',
        icon: 'key-01',
        render: (m) => <KeysSection vm={m} />,
      },
    ]
    if (vm.detail.descriptors?.some((descriptor) => descriptor.kind !== 'source')) {
      sections.push({
        id: 'details',
        label: 'Details',
        icon: 'box',
        render: (m) => <DescriptorList vm={m} />,
      })
    }
    if (showDdl) {
      sections.push(ddlSection)
    }
    sections.push({
      id: 'data',
      label: 'Data',
      icon: 'database',
      render: (m) => <ObjectDataPreview vm={m} />,
    })
    return sections
  }

  const descriptors = vm.detail.descriptors ?? []
  const sections: SectionDef[] = []
  if (descriptors.some((d) => d.kind !== 'source')) {
    sections.push({
      id: 'overview',
      label: 'Overview',
      icon: 'box',
      render: (m) => <KeysSection vm={m} />,
    })
  }
  const inlineSources = descriptors.filter((d) => d.kind === 'source')
  inlineSources.forEach((d, i) => {
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
  if (inlineSources.length === 0 && showDdl) {
    sections.push(ddlSection)
  }
  if (sections.length === 0) {
    sections.push({
      id: 'overview',
      label: 'Overview',
      icon: 'box',
      render: (m) => <KeysSection vm={m} />,
    })
  }
  return sections
}
