import { describe, it, expect } from 'vitest'
import {
  createGroup,
  allGroups,
  findGroup,
  findGroupOfTab,
  firstGroup,
  normalize,
  addTab,
  removeTab,
  setActive,
  setActiveInGroup,
  moveTabBetweenGroups,
  splitGroup,
  splitToEdge,
  removeTabFromGroup,
  migrateToLayout,
  type GroupNode,
  type LayoutNode,
  type SplitNode,
} from './ideLayout'

const row = (children: LayoutNode[]): LayoutNode => ({ type: 'split', id: 's', orientation: 'row', children })

describe('layout read helpers', () => {
  it('lists groups left-to-right', () => {
    const tree = row([createGroup('g1', ['a', 'b'], 'a'), createGroup('g2', ['c'], 'c')])
    expect(allGroups(tree).map((g) => g.id)).toEqual(['g1', 'g2'])
  })
  it('finds a group by id and a group by tab', () => {
    const tree = row([createGroup('g1', ['a']), createGroup('g2', ['b'])])
    expect(findGroup(tree, 'g2')?.id).toBe('g2')
    expect(findGroupOfTab(tree, 'b')?.id).toBe('g2')
    expect(findGroupOfTab(tree, 'zzz')).toBeUndefined()
  })
  it('firstGroup returns the leftmost leaf', () => {
    const tree = row([createGroup('g1', []), createGroup('g2', [])])
    expect(firstGroup(tree).id).toBe('g1')
  })
})

describe('normalize', () => {
  it('drops empty groups that have siblings', () => {
    const tree = row([createGroup('g1', ['a'], 'a'), createGroup('g2', [], undefined)])
    expect(normalize(tree)).toEqual(createGroup('g1', ['a'], 'a'))
  })
  it('unwraps a single-child split', () => {
    const tree = row([createGroup('g1', ['a'], 'a')])
    expect(normalize(tree)).toEqual(createGroup('g1', ['a'], 'a'))
  })
  it('keeps one empty group when everything is empty', () => {
    const tree = row([createGroup('g1', []), createGroup('g2', [])])
    const out = normalize(tree)
    expect(out.type).toBe('group')
    expect((out as GroupNode).tabIds).toEqual([])
  })
  it('leaves a lone empty group as-is', () => {
    const tree = createGroup('g1', [])
    expect(normalize(tree)).toEqual(createGroup('g1', []))
  })
})

describe('tab mutations', () => {
  it('addTab appends to a group and makes it active', () => {
    const tree = createGroup('g1', ['a'], 'a')
    expect(findGroup(addTab(tree, 'g1', 'b'), 'g1')).toEqual(createGroup('g1', ['a', 'b'], 'b'))
  })
  it('removeTab drops the tab and picks a neighbor as active', () => {
    const tree = createGroup('g1', ['a', 'b', 'c'], 'b')
    const g = findGroup(removeTab(tree, 'b'), 'g1')!
    expect(g.tabIds).toEqual(['a', 'c'])
    expect(g.activeTabId).toBe('c')
  })
  it('removeTab removes an emptied group when it has siblings', () => {
    const tree = row([createGroup('g1', ['a'], 'a'), createGroup('g2', ['b'], 'b')])
    expect(removeTab(tree, 'b')).toEqual(createGroup('g1', ['a'], 'a'))
  })
  it('setActive sets the containing group active tab', () => {
    const tree = row([createGroup('g1', ['a', 'b'], 'a'), createGroup('g2', ['c'], 'c')])
    expect(findGroup(setActive(tree, 'b'), 'g1')!.activeTabId).toBe('b')
  })
  it('setActiveInGroup only touches the named group, not other panes of the same tab', () => {
    const tree = row([createGroup('g1', ['a', 'b'], 'b'), createGroup('g2', ['a'], 'a')])
    const out = setActiveInGroup(tree, 'g2', 'a')
    expect(findGroup(out, 'g2')!.activeTabId).toBe('a')
    expect(findGroup(out, 'g1')!.activeTabId).toBe('b') // unchanged
  })
  it('moveTabBetweenGroups reorders within a group (before/after)', () => {
    const tree = createGroup('g1', ['a', 'b', 'c'], 'a')
    expect(findGroup(moveTabBetweenGroups(tree, 'g1', 'a', 'g1', 'c', 'after'), 'g1')!.tabIds).toEqual(['b', 'c', 'a'])
  })
  it('moveTabBetweenGroups moves the source instance only, keeping duplicates elsewhere', () => {
    const tree = row([createGroup('g1', ['a', 'b'], 'a'), createGroup('g2', ['a', 'c'], 'a')])
    // drag the g1 instance of 'b' into g2 before 'c'
    const out = moveTabBetweenGroups(tree, 'g1', 'b', 'g2', 'c', 'before')
    expect(findGroup(out, 'g1')!.tabIds).toEqual(['a'])
    expect(findGroup(out, 'g2')!.tabIds).toEqual(['a', 'b', 'c'])
    expect(findGroup(out, 'g2')!.activeTabId).toBe('b')
  })
})

describe('splitGroup (duplicate beside a specific group)', () => {
  it('duplicates a tab into a new group on the right, source keeps it', () => {
    const tree = createGroup('g1', ['a', 'b'], 'a')
    const { node, newGroupId } = splitGroup(tree, 'g1', 'b', 'right', 'g2')
    const split = node as SplitNode
    expect(split.orientation).toBe('row')
    expect(split.children.map((c) => (c as GroupNode).id)).toEqual(['g1', 'g2'])
    expect(findGroup(node, 'g1')!.tabIds).toEqual(['a', 'b']) // source still has b
    expect(findGroup(node, 'g2')!.tabIds).toEqual(['b'])
    expect(newGroupId).toBe('g2')
  })
  it('splits the SPECIFIC group, not the first group holding the tab', () => {
    // 'a' is in g1 and g2; splitting from g2 must place the new group next to g2.
    const tree = row([createGroup('g1', ['a', 'b'], 'b'), createGroup('g2', ['a'], 'a')])
    const { node } = splitGroup(tree, 'g2', 'a', 'right', 'g3')
    expect((node as SplitNode).children.map((c) => (c as GroupNode).id)).toEqual(['g1', 'g2', 'g3'])
    expect(findGroup(node, 'g3')!.tabIds).toEqual(['a'])
  })
  it('inserts the new group immediately left of the source group', () => {
    const tree = row([createGroup('g1', ['a', 'b'], 'a'), createGroup('g2', ['c', 'd'], 'c'), createGroup('g3', ['e', 'f'], 'e')])
    const { node } = splitGroup(tree, 'g2', 'c', 'left', 'g4')
    expect((node as SplitNode).children.map((c) => (c as GroupNode).id)).toEqual(['g1', 'g4', 'g2', 'g3'])
    expect(findGroup(node, 'g2')!.tabIds).toEqual(['c', 'd']) // source unchanged (duplicated)
  })
  it('can duplicate a lone tab for a same-file side-by-side view', () => {
    const tree = createGroup('g1', ['a'], 'a')
    const { node } = splitGroup(tree, 'g1', 'a', 'right', 'g2')
    expect(findGroup(node, 'g1')!.tabIds).toEqual(['a'])
    expect(findGroup(node, 'g2')!.tabIds).toEqual(['a'])
  })
  it('splitting down wraps a lone group in a column split', () => {
    const tree = createGroup('g1', ['a', 'b'], 'a')
    const { node } = splitGroup(tree, 'g1', 'b', 'down', 'g2')
    const split = node as SplitNode
    expect(split.orientation).toBe('column')
    expect(split.children.map((c) => (c as GroupNode).id)).toEqual(['g1', 'g2'])
  })
  it('splitting up places the new group above (column order)', () => {
    const tree = createGroup('g1', ['a', 'b'], 'a')
    const { node } = splitGroup(tree, 'g1', 'b', 'up', 'g2')
    expect((node as SplitNode).children.map((c) => (c as GroupNode).id)).toEqual(['g2', 'g1'])
  })
  it('splitting a group down inside a row wraps just that group in a nested column', () => {
    const tree = row([createGroup('g1', ['a'], 'a'), createGroup('g2', ['b', 'c'], 'b')])
    const { node } = splitGroup(tree, 'g2', 'c', 'down', 'g3')
    const rootSplit = node as SplitNode
    expect(rootSplit.orientation).toBe('row')
    expect(rootSplit.children[0].type).toBe('group')
    const nested = rootSplit.children[1] as SplitNode
    expect(nested.orientation).toBe('column')
    expect(nested.children.map((c) => (c as GroupNode).id)).toEqual(['g2', 'g3'])
  })
})

describe('splitToEdge', () => {
  it('adds a new group at the far right of the root row', () => {
    const tree = row([createGroup('g1', ['a'], 'a'), createGroup('g2', ['b'], 'b')])
    const { node } = splitToEdge(tree, 'a', 'right', 'g3')
    expect((node as SplitNode).children.map((c) => (c as GroupNode).id)).toEqual(['g1', 'g2', 'g3'])
    expect(findGroup(node, 'g3')!.tabIds).toEqual(['a'])
  })
  it('wraps a lone group into a row when splitting to the left edge', () => {
    const tree = createGroup('g1', ['a'], 'a')
    const { node } = splitToEdge(tree, 'a', 'left', 'g2')
    expect((node as SplitNode).orientation).toBe('row')
    expect((node as SplitNode).children.map((c) => (c as GroupNode).id)).toEqual(['g2', 'g1'])
  })
})

describe('removeTabFromGroup', () => {
  it('removes a tab from one group only, leaving duplicates in other groups', () => {
    const tree = row([createGroup('g1', ['a'], 'a'), createGroup('g2', ['a', 'b'], 'a')])
    const out = removeTabFromGroup(tree, 'g2', 'a')
    expect(findGroup(out, 'g1')!.tabIds).toEqual(['a']) // other instance kept
    expect(findGroup(out, 'g2')!.tabIds).toEqual(['b'])
  })
  it('collapses the group when its last tab is removed', () => {
    const tree = row([createGroup('g1', ['a'], 'a'), createGroup('g2', ['b'], 'b')])
    expect(removeTabFromGroup(tree, 'g2', 'b')).toEqual(createGroup('g1', ['a'], 'a'))
  })
})

describe('migrateToLayout', () => {
  it('builds a single group from a workspace tab list + active tab', () => {
    expect(migrateToLayout('g1', ['a', 'b', 'c'], 'b')).toEqual(createGroup('g1', ['a', 'b', 'c'], 'b'))
  })
})
