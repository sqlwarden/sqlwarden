import type {
  ObjectGroup,
  SchemaDirectory,
  SchemaSpec,
  ScopeNode,
  ScopePath,
} from '#/lib/api/types'

export function kindLabel(spec: SchemaSpec | undefined, kind: string): string {
  return spec?.kinds.find((k) => k.kind === kind)?.plural_label ?? fallbackKindLabel(kind)
}

/** Singular display label for an object kind (e.g. "Materialized view"), used
 *  in confirmations and dialogs where the plural directory label reads oddly. */
export function kindLabelSingular(spec: SchemaSpec | undefined, kind: string): string {
  const found = spec?.kinds.find((k) => k.kind === kind)?.label
  if (found) return found
  return kind
    .split('_')
    .filter(Boolean)
    .map((word, i) => (i === 0 ? word.charAt(0).toUpperCase() + word.slice(1) : word))
    .join(' ')
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

/** Whether an object kind has row/column detail worth expanding, per the
 *  backend schema spec. Kinds absent from the spec (or before it loads)
 *  default to relational so the tree doesn't hide detail it hasn't ruled out
 *  yet. */
export function isRelationalKind(spec: SchemaSpec | undefined, kind: string): boolean {
  return spec?.kinds.find((k) => k.kind === kind)?.relational ?? true
}

export function sortedGroups(node: ScopeNode, spec: SchemaSpec | undefined): ObjectGroup[] {
  const byKind = new Map(node.groups.map((group) => [group.kind, group]))
  for (const specKind of spec?.kinds ?? []) {
    if (!byKind.has(specKind.kind)) byKind.set(specKind.kind, { kind: specKind.kind, objects: [] })
  }
  return [...byKind.values()].sort(
    (a, b) => kindOrder(spec, a.kind) - kindOrder(spec, b.kind) || a.kind.localeCompare(b.kind),
  )
}

/** Compact row-count label for the schema tree (e.g. "~1.2K"), matching the
 *  tilde-prefixed convention DBeaver uses for catalog-derived estimates. */
export function formatRowCount(count: number): string {
  if (count < 1000) return `${count}`
  const [divisor, suffix] = count < 1_000_000 ? [1_000, 'K'] : [1_000_000, 'M']
  return `~${(count / divisor).toFixed(1)}${suffix}`
}

export function hasDirectoryObjects(nodes: ScopeNode[]): boolean {
  return nodes.some(
    (node) =>
      node.groups.some((group) => group.objects.length > 0) ||
      hasDirectoryObjects(node.children ?? []),
  )
}

/** Best-effort scope for a first "Create table" action when a database has no
 *  objects yet: the sole root scope, or its sole child scope when the root is
 *  just a container (e.g. a database holding a single schema). Ambiguous
 *  shapes (multiple roots, multiple children) return null rather than
 *  guessing which scope the user meant. */
export function defaultCreateTableScope(roots: ScopeNode[]): ScopePath | null {
  if (roots.length !== 1) return null
  const children = roots[0].children ?? []
  if (children.length === 0) return roots[0].path
  if (children.length === 1) return children[0].path
  return null
}

/** Fixes up null slices. Exposed separately from filterDirectory so a caller
 *  that filters repeatedly (e.g. per keystroke) can memoize normalization
 *  independently of the query. */
export function normalizeDirectory(directory: SchemaDirectory): SchemaDirectory {
  const roots = normalizeNodes(directory.roots)
  return roots === directory.roots ? directory : { ...directory, roots }
}

/** Filters an already-normalized directory by query. */
export function filterNormalized(directory: SchemaDirectory, query: string): SchemaDirectory {
  const q = query.trim().toLowerCase()
  if (!q) return directory
  return { ...directory, roots: filterNodes(directory.roots, q) }
}

export function filterDirectory(directory: SchemaDirectory, query: string): SchemaDirectory {
  return filterNormalized(normalizeDirectory(directory), query)
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

/** Lowercased names keyed by object identity. normalizeNodes preserves node,
 *  group, and segment identity when unchanged, so entries persist across
 *  repeated filtering of the same snapshot. */
const lowerNameCache = new WeakMap<{ name: string }, string>()

function lowerName(named: { name: string }): string {
  let cached = lowerNameCache.get(named)
  if (cached === undefined) {
    cached = named.name.toLowerCase()
    lowerNameCache.set(named, cached)
  }
  return cached
}

function pathMatches(path: ScopePath, query: string): boolean {
  return path.some((segment) => lowerName(segment).includes(query))
}

/** Lowercase text of everything under a scope node — its own path segments,
 *  every object name in every group, and all descendant scopes — joined into
 *  one string and cached per node identity. Lets filtering test "could this
 *  subtree possibly match?" with a single string search instead of walking
 *  and reallocating the whole subtree on every keystroke; normalizeNodes
 *  preserves node identity for unchanged data, so the cache survives across
 *  filter passes of the same directory. */
const subtreeSignatureCache = new WeakMap<ScopeNode, string>()

function subtreeSignature(node: ScopeNode): string {
  const cached = subtreeSignatureCache.get(node)
  if (cached !== undefined) return cached
  const parts: string[] = node.path.map((segment) => lowerName(segment))
  for (const group of node.groups) {
    for (const object of group.objects) parts.push(lowerName(object))
  }
  for (const child of node.children ?? []) parts.push(subtreeSignature(child))
  const signature = parts.join(' ')
  subtreeSignatureCache.set(node, signature)
  return signature
}

function filterNodes(nodes: ScopeNode[], query: string): ScopeNode[] {
  const result: ScopeNode[] = []
  for (const node of nodes) {
    if (pathMatches(node.path, query)) {
      result.push(node)
      continue
    }
    // No match anywhere in this subtree: skip it without allocating a single
    // filtered group/child, instead of the old map+filter that rebuilt every
    // subtree (matching or not) on every pass.
    if (!subtreeSignature(node).includes(query)) continue
    const groups = node.groups
      .map((group) => ({
        ...group,
        objects: group.objects.filter((object) => lowerName(object).includes(query)),
      }))
      .filter((group) => group.objects.length > 0)
    const children = filterNodes(node.children ?? [], query)
    if (groups.length > 0 || children.length > 0) result.push({ ...node, groups, children })
  }
  return result
}
