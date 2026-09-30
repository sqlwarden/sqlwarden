import { describe, expect, it } from 'vitest'
import type { NavigatorNode, NavigatorTree } from '#/lib/api/types'
import { diagramFolders, diagramSupported, diagramSupportedForKind } from './capability'

function node(overrides: Partial<NavigatorNode> = {}): NavigatorNode {
  return {
    label: '',
    icon: 'box',
    leaf: false,
    folders: [],
    scope: false,
    relational: false,
    supports_diagram: false,
    has_definition: false,
    show_all_databases: false,
    column: false,
    ...overrides,
  }
}

const tree: NavigatorTree = {
  system_objects: false,
  root: node({
    folders: [{ kind: 'databases', label: 'Databases', child: 'database', order: 10 }],
  }),
  nodes: {
    database: node({ scope: true }),
    schema: node({
      scope: true,
      folders: [
        { kind: 'tables', label: 'Tables', child: 'table', order: 10 },
        { kind: 'functions', label: 'Functions', child: 'function', order: 20 },
      ],
    }),
    table: node({ relational: true, supports_diagram: true }),
    function: node({ leaf: true }),
  },
}

describe('diagram capability', () => {
  it('is supported when any grammar node supports a diagram', () => {
    expect(diagramSupported(tree)).toBe(true)
    expect(diagramSupported(undefined)).toBe(false)
    expect(diagramSupported({ ...tree, nodes: { function: node({ leaf: true }) } })).toBe(false)
  })

  it('checks a specific kind', () => {
    expect(diagramSupportedForKind(tree, 'table')).toBe(true)
    expect(diagramSupportedForKind(tree, 'function')).toBe(false)
    expect(diagramSupportedForKind(tree, 'missing')).toBe(false)
  })

  it('returns the scope folders whose children can be diagrammed', () => {
    const scope = [
      { kind: 'database', name: 'app' },
      { kind: 'schema', name: 'public' },
    ]
    expect(diagramFolders(tree, scope).map((f) => f.kind)).toEqual(['tables'])
    expect(diagramFolders(tree, [{ kind: 'database', name: 'app' }])).toEqual([])
    expect(diagramFolders(undefined, scope)).toEqual([])
  })
})
