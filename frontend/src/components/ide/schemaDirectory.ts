import type { ObjectGroup, SchemaDirectory, SchemaSpec, ScopeNode } from '#/lib/api/types'

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

export function sortedGroups(node: ScopeNode, spec: SchemaSpec | undefined): ObjectGroup[] {
  return [...node.groups].sort(
    (a, b) => kindOrder(spec, a.kind) - kindOrder(spec, b.kind) || a.kind.localeCompare(b.kind),
  )
}

export function hasDirectoryObjects(nodes: ScopeNode[]): boolean {
  return nodes.some(
    (node) =>
      node.groups.some((group) => group.objects.length > 0) ||
      hasDirectoryObjects(node.children ?? []),
  )
}

export function filterDirectory(directory: SchemaDirectory, query: string): SchemaDirectory {
  const roots = normalizeNodes(directory.roots)
  const normalized = roots === directory.roots ? directory : { ...directory, roots }
  const q = query.trim().toLowerCase()
  if (!q) return normalized

  return { ...normalized, roots: filterNodes(roots, q) }
}

/**
 * Older snapshots and empty Go slices may contain explicit JSON nulls even
 * though the API contract exposes collections. Normalize at this boundary so
 * every schema-tree consumer can safely treat them as arrays.
 */
function normalizeNodes(nodes: ScopeNode[] | null | undefined): ScopeNode[] {
  if (!nodes) return []

  let changed = false
  const normalized = nodes.map((node) => {
    const groups = normalizeGroups(node.groups)
    const children = node.children == null ? undefined : normalizeNodes(node.children)
    if (groups === node.groups && children === node.children) return node
    changed = true
    return { ...node, groups, children }
  })
  return changed ? normalized : nodes
}

function normalizeGroups(groups: ObjectGroup[] | null | undefined): ObjectGroup[] {
  if (!groups) return []

  let changed = false
  const normalized = groups.map((group) => {
    if (group.objects) return group
    changed = true
    return { ...group, objects: [] }
  })
  return changed ? normalized : groups
}

function filterNodes(nodes: ScopeNode[], query: string): ScopeNode[] {
  return nodes
    .map((node) => {
      const groups = node.groups
        .map((group) => ({
          ...group,
          objects: group.objects.filter((object) => object.name.toLowerCase().includes(query)),
        }))
        .filter((group) => group.objects.length > 0)
      const children = filterNodes(node.children ?? [], query)
      return { ...node, groups, children }
    })
    .filter((node) => node.groups.length > 0 || (node.children?.length ?? 0) > 0)
}
