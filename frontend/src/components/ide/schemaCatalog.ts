import type { CatalogNamespace, CatalogObjectGroup, SchemaCatalog, SchemaSpec } from '#/lib/api/types'

export function kindLabel(spec: SchemaSpec | undefined, kind: string): string {
  return spec?.kinds.find((k) => k.kind === kind)?.plural_label ?? fallbackKindLabel(kind)
}

function fallbackKindLabel(kind: string): string {
  const words = kind.split('_').filter(Boolean)
  if (words.length === 0) return kind

  const last = words[words.length - 1]
  words[words.length - 1] = last.endsWith('s') ? last : `${last}s`
  return words.map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(' ')
}

function kindOrder(spec: SchemaSpec | undefined, kind: string): number {
  return spec?.kinds.find((k) => k.kind === kind)?.order ?? Number.MAX_SAFE_INTEGER
}

export function sortedGroups(ns: CatalogNamespace, spec: SchemaSpec | undefined): CatalogObjectGroup[] {
  return [...(ns.groups ?? [])].sort(
    (a, b) => kindOrder(spec, a.kind) - kindOrder(spec, b.kind) || a.kind.localeCompare(b.kind),
  )
}

export function filterCatalog(catalog: SchemaCatalog, query: string): SchemaCatalog {
  const q = query.trim().toLowerCase()
  if (!q) return catalog

  const namespaces = (catalog.namespaces ?? [])
    .map((ns) => filterNamespace(ns, q))
    .filter((ns) => (ns.groups ?? []).length > 0)

  return { ...catalog, namespaces }
}

function filterNamespace(ns: CatalogNamespace, q: string): CatalogNamespace {
  const groups = (ns.groups ?? [])
    .map((g) => ({ ...g, objects: g.objects.filter((o) => o.name.toLowerCase().includes(q)) }))
    .filter((g) => g.objects.length > 0)
  return { ...ns, groups }
}
