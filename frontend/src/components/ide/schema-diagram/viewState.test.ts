import { describe, expect, it } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import { resolveDiagramViewState } from './viewState'

const base = {
  hasTarget: true,
  hasConnection: true,
  hasSession: true,
  loadRequested: true,
  tree: {
    system_objects: false,
    root: {
      label: 'Connection',
      icon: 'connection',
      leaf: false,
      folders: [],
      scope: false,
      relational: false,
      supports_diagram: false,
      has_definition: false,
      show_all_databases: false,
      column: false,
    },
    nodes: {
      table: {
        label: 'Table',
        icon: 'table',
        leaf: false,
        folders: [],
        scope: false,
        relational: true,
        supports_diagram: true,
        has_definition: true,
        show_all_databases: false,
        column: false,
      },
    },
  },
  treeError: null,
  listingError: null,
  relationshipsError: null,
  listingLoading: false,
  relationshipsLoading: false,
  presentCount: 1,
}

describe('resolveDiagramViewState', () => {
  it('prioritizes malformed tabs and missing sessions', () => {
    expect(resolveDiagramViewState({ ...base, hasTarget: false })).toBe('missing-target')
    expect(
      resolveDiagramViewState({
        ...base,
        hasSession: false,
        relationshipsError: new ApiError('Forbidden', 403),
      }),
    ).toBe('no-session')
    expect(resolveDiagramViewState({ ...base, loadRequested: false })).toBe('needs-load')
  })

  it('distinguishes unsupported capabilities and authorization loss', () => {
    expect(
      resolveDiagramViewState({ ...base, relationshipsError: new ApiError('Unsupported', 501) }),
    ).toBe('unsupported')
    expect(resolveDiagramViewState({ ...base, tree: { ...base.tree, nodes: {} } })).toBe(
      'unsupported',
    )
    expect(resolveDiagramViewState({ ...base, listingError: new ApiError('Forbidden', 403) })).toBe(
      'forbidden',
    )
  })

  it('orders loading and empty states after terminal query states', () => {
    expect(resolveDiagramViewState({ ...base, listingLoading: true })).toBe('loading')
    expect(resolveDiagramViewState({ ...base, presentCount: 0 })).toBe('empty')
    expect(resolveDiagramViewState(base)).toBe('ready')
  })
})
