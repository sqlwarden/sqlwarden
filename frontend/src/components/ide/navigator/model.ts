import type {
  NavigatorFolder,
  NavigatorItem,
  NavigatorListing,
  NavigatorNode,
  NavigatorTree,
  ObjectRef,
  ScopePath,
} from '#/lib/api/types'

export type ListingState =
  | { status: 'loading' }
  | { status: 'session_required' }
  | { status: 'error' }
  | { status: 'ready'; listing: NavigatorListing }

export type NavigatorRow =
  | {
      type: 'folder'
      key: string
      depth: number
      parent: ScopePath
      folder: NavigatorFolder
      expanded: boolean
      listing?: NavigatorListing
    }
  | {
      type: 'object'
      key: string
      depth: number
      item: NavigatorItem
      node: NavigatorNode
      folder: NavigatorFolder
      expanded: boolean
    }
  | {
      type: 'leaf'
      key: string
      depth: number
      item: NavigatorItem
      node: NavigatorNode | undefined
      folder: NavigatorFolder
    }
  | {
      type: 'status'
      key: string
      depth: number
      parent: ScopePath
      folder: NavigatorFolder
      status: 'loading' | 'session_required' | 'error' | 'empty'
    }

export interface FolderRequest {
  parent: ScopePath
  folder: string
}

export interface FlattenInput {
  tree: NavigatorTree
  isExpanded: (key: string) => boolean
  listing: (parent: ScopePath, folder: string) => ListingState | undefined
  filter?: string
}

export function folderKey(parent: ScopePath, folder: string) {
  return `folder:${JSON.stringify(parent)}:${folder}`
}

export function objectKey(path: ScopePath) {
  return `object:${JSON.stringify(path)}`
}

export function refOfPath(path: ScopePath): ObjectRef {
  const last = path[path.length - 1]
  return { scope: path.slice(0, -1), kind: last.kind, name: last.name }
}

/**
 * Flattens the expanded navigator into render rows plus the folder listings the
 * caller must fetch. Filtering reads `listing` for collapsed folders too, but
 * only ever reveals ready listings and never adds them to `requests`.
 */
export function flattenNavigator({ tree, isExpanded, listing, filter = '' }: FlattenInput) {
  const rows: NavigatorRow[] = []
  const requests: FolderRequest[] = []
  const usedKeys = new Set<string>()
  const matchMemo = new Map<string, boolean>()

  const uniqueKey = (key: string) => {
    let candidate = key
    for (let n = 1; usedKeys.has(candidate); n++) candidate = `${key}#${n}`
    usedKeys.add(candidate)
    return candidate
  }

  const cached = (parent: ScopePath, folder: string) => {
    const state = listing(parent, folder)
    return state?.status === 'ready' ? state.listing : undefined
  }

  const nameMatches = (item: NavigatorItem, query: string) =>
    item.name.toLowerCase().includes(query)

  function folderHasMatch(parent: ScopePath, folder: NavigatorFolder, query: string): boolean {
    const memoKey = `${folderKey(parent, folder.kind)}|${query}`
    const memo = matchMemo.get(memoKey)
    if (memo !== undefined) return memo
    const items = cached(parent, folder.kind)?.items ?? []
    const result = items.some((item) => itemMatches(item, query))
    matchMemo.set(memoKey, result)
    return result
  }

  function itemMatches(item: NavigatorItem, query: string): boolean {
    if (nameMatches(item, query)) return true
    const node = tree.nodes[item.kind]
    if (!node || node.leaf) return false
    return node.folders.some((folder) => folderHasMatch(item.path, folder, query))
  }

  function visitFolders(parent: ScopePath, node: NavigatorNode, depth: number, query: string) {
    for (const folder of node.folders) {
      const key = folderKey(parent, folder.kind)
      const userExpanded = isExpanded(key)
      if (query !== '' && !folderHasMatch(parent, folder, query)) continue
      const expanded = userExpanded || query !== ''
      const state = expanded ? listing(parent, folder.kind) : undefined
      rows.push({
        type: 'folder',
        key: uniqueKey(key),
        depth,
        parent,
        folder,
        expanded,
        listing: state?.status === 'ready' ? state.listing : undefined,
      })
      if (!expanded) continue
      if (userExpanded) requests.push({ parent, folder: folder.kind })

      const status = (value: 'loading' | 'session_required' | 'error' | 'empty') =>
        rows.push({
          type: 'status',
          key: uniqueKey(`status:${key}`),
          depth: depth + 1,
          parent,
          folder,
          status: value,
        })

      if (!state || state.status === 'loading') {
        status('loading')
        continue
      }
      if (state.status !== 'ready') {
        status(state.status)
        continue
      }
      const items =
        query === ''
          ? state.listing.items
          : state.listing.items.filter((item) => itemMatches(item, query))
      if (items.length === 0) {
        status('empty')
        continue
      }
      for (const item of items) visitItem(item, folder, depth + 1, query)
    }
  }

  function visitItem(item: NavigatorItem, folder: NavigatorFolder, depth: number, query: string) {
    const node = tree.nodes[item.kind]
    if (!node || node.leaf) {
      rows.push({
        type: 'leaf',
        key: uniqueKey(`leaf:${JSON.stringify(item.path)}`),
        depth,
        item,
        node,
        folder,
      })
      return
    }
    const key = objectKey(item.path)
    const childQuery = query !== '' && nameMatches(item, query) ? '' : query
    const expanded = isExpanded(key) || (childQuery !== '' && itemMatches(item, childQuery))
    rows.push({ type: 'object', key: uniqueKey(key), depth, item, node, folder, expanded })
    if (expanded) visitFolders(item.path, node, depth + 1, childQuery)
  }

  visitFolders([], tree.root, 0, filter.trim().toLowerCase())
  return { rows, requests }
}
