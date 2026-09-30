import type { NavigatorFolder, NavigatorTree, ScopePath } from '#/lib/api/types'

export function diagramSupported(tree: NavigatorTree | undefined): boolean {
  return Boolean(tree && Object.values(tree.nodes).some((node) => node.supports_diagram))
}

export function diagramSupportedForKind(tree: NavigatorTree | undefined, kind: string): boolean {
  return tree?.nodes[kind]?.supports_diagram === true
}

/** Folders directly under the object at `scope` whose children can appear on a diagram. */
export function diagramFolders(
  tree: NavigatorTree | undefined,
  scope: ScopePath,
): NavigatorFolder[] {
  if (!tree) return []
  const owner = scope.length === 0 ? tree.root : tree.nodes[scope[scope.length - 1].kind]
  return (owner?.folders ?? []).filter((folder) => tree.nodes[folder.child]?.supports_diagram)
}
