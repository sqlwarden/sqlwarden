import type { PropsWithChildren } from 'react'
import { render, screen } from '@testing-library/react'
import { createRootRoute } from '@tanstack/react-router'
import { describe, expect, it, vi } from 'vitest'

vi.mock('./build', () => ({
  distribution: {
    providers: [
      ({ children }: PropsWithChildren) => <div data-testid="paid-provider">{children}</div>,
    ],
    routes: [{ scope: 'root', path: 'paid', component: () => <div>Paid route</div> }],
    navigation: [
      {
        scope: 'organization',
        section: 'Security',
        permission: 'approval:read',
        item: ({ orgSlug }: { orgSlug?: string }) => ({
          to: `/orgs/${orgSlug}/approvals`,
          label: 'Approvals',
          icon: 'shield-user',
        }),
      },
    ],
  },
}))

import {
  composeDistributionRoutes,
  distributionNavigationGroups,
  DistributionProviders,
} from './composition'

describe('distribution composition', () => {
  it('wraps Community providers at build time', () => {
    render(<DistributionProviders>Community</DistributionProviders>)
    expect(screen.getByTestId('paid-provider')).toHaveTextContent('Community')
  })

  it('adds contributed routes to their selected parent', () => {
    const root = createRootRoute()
    composeDistributionRoutes({
      root,
      account: root,
      instance: root,
      organization: root,
      workspace: root,
    })
    const children = (root as unknown as { children?: unknown[] }).children
    expect(children).toHaveLength(1)
  })

  it('filters and resolves contributed navigation', () => {
    expect(
      distributionNavigationGroups('organization', { orgSlug: 'acme', permissions: [] }).size,
    ).toBe(0)
    const groups = distributionNavigationGroups('organization', {
      orgSlug: 'acme',
      permissions: ['approval:read'],
    })
    expect(groups.get('Security')).toEqual([
      expect.objectContaining({ to: '/orgs/acme/approvals', label: 'Approvals' }),
    ])
  })
})
