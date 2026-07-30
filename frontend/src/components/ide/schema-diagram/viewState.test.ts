import { describe, expect, it } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import { resolveDiagramViewState } from './viewState'

const base = {
  hasTarget: true,
  hasConnection: true,
  hasSession: true,
  spec: {
    dialect: 'postgres',
    kinds: [
      {
        kind: 'table',
        label: 'Table',
        plural_label: 'Tables',
        order: 1,
        relational: true,
        supports_diagram: true,
        listing: 'enumerated' as const,
      },
    ],
  },
  specError: null,
  directoryError: null,
  relationshipsError: null,
  directoryLoading: false,
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
  })

  it('distinguishes unsupported capabilities and authorization loss', () => {
    expect(
      resolveDiagramViewState({ ...base, relationshipsError: new ApiError('Unsupported', 501) }),
    ).toBe('unsupported')
    expect(resolveDiagramViewState({ ...base, spec: { dialect: 'redis', kinds: [] } })).toBe(
      'unsupported',
    )
    expect(resolveDiagramViewState({ ...base, directoryError: new ApiError('Forbidden', 403) })).toBe(
      'forbidden',
    )
  })

  it('orders loading and empty states after terminal query states', () => {
    expect(resolveDiagramViewState({ ...base, directoryLoading: true })).toBe('loading')
    expect(resolveDiagramViewState({ ...base, presentCount: 0 })).toBe('empty')
    expect(resolveDiagramViewState(base)).toBe('ready')
  })
})
