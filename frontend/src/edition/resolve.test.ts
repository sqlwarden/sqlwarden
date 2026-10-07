import { describe, expect, it } from 'vitest'
import type { EditionCapabilities, EditionFeatureState } from '#/lib/api/types'
import { lockedEditionNavigation, resolveEditionPage } from './resolve'
import { emptyEditionRegistry, type EditionRegistry } from './types'

function capabilities(state: EditionFeatureState, navigation: string | null = null) {
  return {
    edition: 'community',
    features: [
      {
        key: 'audit.tamper_evidence',
        label: 'Tamper-evident audit',
        description: 'Hash-chained audit evidence.',
        docs_url: null,
        navigation,
        state,
      },
    ],
  } satisfies EditionCapabilities
}

const Page = () => null

function registryWithAuditPage(): EditionRegistry {
  return {
    ...emptyEditionRegistry(),
    routes: [{ id: 'audit', feature: 'audit.tamper_evidence', path: 'audit', component: Page }],
  }
}

describe('resolveEditionPage', () => {
  it('renders a registered page when its feature is available', () => {
    expect(resolveEditionPage(registryWithAuditPage(), capabilities('available'), 'audit')).toEqual(
      { kind: 'route', component: Page },
    )
  })

  it.each(['upgrade', 'unlicensed'] as const)('locks a registered page in state %s', (state) => {
    const page = resolveEditionPage(registryWithAuditPage(), capabilities(state), 'audit')
    expect(page.kind).toBe('locked')
  })

  it('locks a catalog feature that has no page in this build', () => {
    const page = resolveEditionPage(
      emptyEditionRegistry(),
      capabilities('upgrade'),
      'audit.tamper_evidence',
    )
    expect(page).toMatchObject({ kind: 'locked', feature: { key: 'audit.tamper_evidence' } })
  })

  it('does not invent a page for an available feature without a route', () => {
    expect(
      resolveEditionPage(
        emptyEditionRegistry(),
        capabilities('available'),
        'audit.tamper_evidence',
      ),
    ).toEqual({ kind: 'not-found' })
  })

  it('returns not found for unknown paths', () => {
    expect(resolveEditionPage(emptyEditionRegistry(), capabilities('upgrade'), 'nope')).toEqual({
      kind: 'not-found',
    })
  })
})

describe('lockedEditionNavigation', () => {
  it('lists only placed features that are not available', () => {
    expect(lockedEditionNavigation(capabilities('upgrade', 'organization'))).toEqual([
      {
        key: 'audit.tamper_evidence',
        label: 'Tamper-evident audit',
        placement: 'organization',
        to: '/ee/audit.tamper_evidence',
      },
    ])
    expect(lockedEditionNavigation(capabilities('available', 'organization'))).toEqual([])
    expect(lockedEditionNavigation(capabilities('upgrade'))).toEqual([])
  })
})
