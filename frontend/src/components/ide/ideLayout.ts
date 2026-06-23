// Pure, React-free helpers for the per-workspace editor layout tree.
// A workspace's editor area is a tree of SplitNode (orientation + children) and
// GroupNode (an ordered list of tab ids + the active tab). A tab can appear in
// more than one group (a duplicated side-by-side pane), so per-pane operations
// address a specific (groupId, tabId) instance, not just a tab id.

export type GroupNode = {
  type: 'group'
  id: string
  tabIds: string[]
  activeTabId?: string
}

export type SplitNode = {
  type: 'split'
  id: string
  orientation: 'row' | 'column'
  children: LayoutNode[]
  sizes?: number[]
}

export type LayoutNode = GroupNode | SplitNode

export type SplitDirection = 'left' | 'right' | 'up' | 'down'

export function createGroup(id: string, tabIds: string[], activeTabId?: string): GroupNode {
  return { type: 'group', id, tabIds, activeTabId: activeTabId ?? tabIds[tabIds.length - 1] }
}

export function allGroups(node: LayoutNode): GroupNode[] {
  return node.type === 'group' ? [node] : node.children.flatMap(allGroups)
}

export function findGroup(node: LayoutNode, groupId: string): GroupNode | undefined {
  return allGroups(node).find((g) => g.id === groupId)
}

export function findGroupOfTab(node: LayoutNode, tabId: string): GroupNode | undefined {
  return allGroups(node).find((g) => g.tabIds.includes(tabId))
}

export function firstGroup(node: LayoutNode): GroupNode {
  return allGroups(node)[0]
}

/** Apply fn to every group, returning a new tree (immutable). */
function mapGroups(node: LayoutNode, fn: (g: GroupNode) => GroupNode): LayoutNode {
  if (node.type === 'group') return fn(node)
  return { ...node, children: node.children.map((c) => mapGroups(c, fn)) }
}

/** Remove empty groups (keeping at least one), unwrap single-child splits. */
export function normalize(node: LayoutNode): LayoutNode {
  if (node.type === 'group') return node
  const children = node.children
    .map(normalize)
    .filter((c) => !(c.type === 'group' && c.tabIds.length === 0))
  if (children.length === 0) {
    const firstEmpty = allGroups(node)[0]
    return createGroup(firstEmpty?.id ?? node.id, [])
  }
  if (children.length === 1) return children[0]
  return { ...node, children }
}

export function addTab(node: LayoutNode, groupId: string, tabId: string, index?: number): LayoutNode {
  return mapGroups(node, (g) => {
    if (g.id !== groupId || g.tabIds.includes(tabId)) return g
    const tabIds = [...g.tabIds]
    tabIds.splice(index ?? tabIds.length, 0, tabId)
    return { ...g, tabIds, activeTabId: tabId }
  })
}

/** Remove a tab from every group that holds it (full close). */
export function removeTab(node: LayoutNode, tabId: string): LayoutNode {
  const updated = mapGroups(node, (g) => {
    if (!g.tabIds.includes(tabId)) return g
    const i = g.tabIds.indexOf(tabId)
    const tabIds = g.tabIds.filter((t) => t !== tabId)
    const activeTabId = g.activeTabId === tabId ? (tabIds[i] ?? tabIds[i - 1]) : g.activeTabId
    return { ...g, tabIds, activeTabId }
  })
  return normalize(updated)
}

/** Remove a tab from one specific group only (duplicates in other groups stay). */
export function removeTabFromGroup(node: LayoutNode, groupId: string, tabId: string): LayoutNode {
  const updated = mapGroups(node, (g) => {
    if (g.id !== groupId || !g.tabIds.includes(tabId)) return g
    const i = g.tabIds.indexOf(tabId)
    const tabIds = g.tabIds.filter((t) => t !== tabId)
    const activeTabId = g.activeTabId === tabId ? (tabIds[i] ?? tabIds[i - 1]) : g.activeTabId
    return { ...g, tabIds, activeTabId }
  })
  return normalize(updated)
}

/** Set the active tab on every group that holds it (used when opening a fresh tab). */
export function setActive(node: LayoutNode, tabId: string): LayoutNode {
  return mapGroups(node, (g) => (g.tabIds.includes(tabId) ? { ...g, activeTabId: tabId } : g))
}

/** Set the active tab on one specific group only (other panes of the same tab unaffected). */
export function setActiveInGroup(node: LayoutNode, groupId: string, tabId: string): LayoutNode {
  return mapGroups(node, (g) => (g.id === groupId && g.tabIds.includes(tabId) ? { ...g, activeTabId: tabId } : g))
}

/**
 * Move a tab instance from one group to another (or reorder within a group),
 * addressing the specific pane by id. Only the source group's instance moves;
 * duplicates of the tab in other groups are left untouched. Returns the original
 * node reference when nothing changes, so repeated dragover events don't churn.
 */
export function moveTabBetweenGroups(
  node: LayoutNode,
  fromGroupId: string,
  draggedTabId: string,
  toGroupId: string,
  targetTabId: string,
  position: 'before' | 'after',
): LayoutNode {
  if (fromGroupId === toGroupId && draggedTabId === targetTabId) return node
  // 1) remove the dragged instance from its source group only
  let next = mapGroups(node, (g) => {
    if (g.id !== fromGroupId || !g.tabIds.includes(draggedTabId)) return g
    const tabIds = g.tabIds.filter((t) => t !== draggedTabId)
    const activeTabId = g.activeTabId === draggedTabId ? g.tabIds.find((t) => t !== draggedTabId) : g.activeTabId
    return { ...g, tabIds, activeTabId }
  })
  // 2) insert into the target group relative to the target tab; make it active there
  next = mapGroups(next, (g) => {
    if (g.id !== toGroupId) return g
    const tabIds = g.tabIds.filter((t) => t !== draggedTabId) // drop a pre-existing duplicate in the target
    let idx = tabIds.indexOf(targetTabId)
    if (idx === -1) idx = tabIds.length
    else if (position === 'after') idx += 1
    tabIds.splice(idx, 0, draggedTabId)
    return { ...g, tabIds, activeTabId: draggedTabId }
  })
  const result = normalize(next)
  const before = allGroups(node)
  const after = allGroups(result)
  const unchanged =
    before.length === after.length &&
    before.every(
      (g, i) =>
        g.id === after[i].id &&
        g.tabIds.join(' ') === after[i].tabIds.join(' ') &&
        g.activeTabId === after[i].activeTabId,
    )
  return unchanged ? node : result
}

function directionToSplit(dir: SplitDirection): { orientation: 'row' | 'column'; before: boolean } {
  switch (dir) {
    case 'left': return { orientation: 'row', before: true }
    case 'right': return { orientation: 'row', before: false }
    case 'up': return { orientation: 'column', before: true }
    case 'down': return { orientation: 'column', before: false }
  }
}

/**
 * Duplicate tabId into a brand-new group placed beside the SPECIFIC source group
 * (addressed by id) in the given direction. The source group keeps the tab, so the
 * same tab (and its synced Y.Doc) shows in both panes. When the source's parent
 * already runs along the requested axis the new group is inserted as a sibling;
 * otherwise the source group is wrapped in a perpendicular split (enables nesting).
 */
export function splitGroup(
  node: LayoutNode,
  srcGroupId: string,
  tabId: string,
  direction: SplitDirection,
  newGroupId: string,
): { node: LayoutNode; newGroupId: string } {
  const src = findGroup(node, srcGroupId)
  if (!src || !src.tabIds.includes(tabId)) return { node, newGroupId }
  const { orientation, before } = directionToSplit(direction)
  const newGroup = createGroup(newGroupId, [tabId], tabId)

  const wrap = (target: LayoutNode): LayoutNode => ({
    type: 'split',
    id: `s-${newGroupId}`,
    orientation,
    children: before ? [newGroup, target] : [target, newGroup],
  })

  if (node.type === 'group') return { node: wrap(node), newGroupId }

  const transform = (n: LayoutNode): LayoutNode => {
    if (n.type === 'group') return n
    const idx = n.children.findIndex((c) => c.type === 'group' && c.id === srcGroupId)
    if (idx !== -1) {
      if (n.orientation === orientation) {
        const children = [...n.children]
        children.splice(before ? idx : idx + 1, 0, newGroup)
        return { ...n, children }
      }
      const children = [...n.children]
      children[idx] = wrap(children[idx])
      return { ...n, children }
    }
    return { ...n, children: n.children.map(transform) }
  }
  return { node: normalize(transform(node)), newGroupId }
}

/** Duplicate tabId into a new group at the far left/right of the root row (edge drop). */
export function splitToEdge(
  node: LayoutNode,
  tabId: string,
  side: 'left' | 'right',
  newGroupId: string,
): { node: LayoutNode; newGroupId: string } {
  if (!findGroupOfTab(node, tabId)) return { node, newGroupId }
  const newGroup = createGroup(newGroupId, [tabId], tabId)
  if (node.type === 'split' && node.orientation === 'row') {
    return {
      node: { ...node, children: side === 'right' ? [...node.children, newGroup] : [newGroup, ...node.children] },
      newGroupId,
    }
  }
  return {
    node: {
      type: 'split',
      id: `s-${newGroupId}`,
      orientation: 'row',
      children: side === 'right' ? [node, newGroup] : [newGroup, node],
    },
    newGroupId,
  }
}

/** Build a default single-group layout for a workspace (used by migration). */
export function migrateToLayout(groupId: string, tabIds: string[], activeTabId?: string): LayoutNode {
  return createGroup(groupId, tabIds, activeTabId)
}

export type TabCloseScope = 'others' | 'right' | 'all'

/** Tab ids a "Close others / to the right / all" action should close, relative
 *  to `targetId` within the ordered `tabIds` of one group. */
export function tabsToClose(scope: TabCloseScope, tabIds: string[], targetId: string): string[] {
  switch (scope) {
    case 'others':
      return tabIds.filter((id) => id !== targetId)
    case 'all':
      return [...tabIds]
    case 'right': {
      const i = tabIds.indexOf(targetId)
      return i === -1 ? [] : tabIds.slice(i + 1)
    }
  }
}
