import { describe, expect, it } from 'vitest'
import {
  isNavItemActive,
  navItemKey,
  resolveNavPath,
  type AppShellNavItem,
} from './app-shell-navigation'

const item: AppShellNavItem = {
  to: '/orgs/$org_slug/workspaces/$workspace_id',
  params: { org_slug: 'acme', workspace_id: '3' },
  label: 'Workspace',
  icon: 'database',
  activePathPrefixes: ['/orgs/acme/workspaces/3'],
}

describe('app shell navigation', () => {
  it('resolves route params and ignores trailing slashes', () => {
    expect(resolveNavPath(item.to, item.params ?? {})).toBe('/orgs/acme/workspaces/3')
    expect(isNavItemActive('/orgs/acme/workspaces/3/', item)).toBe(true)
  })

  it('matches configured descendants without matching sibling prefixes', () => {
    expect(isNavItemActive('/orgs/acme/workspaces/3/users', item)).toBe(true)
    expect(isNavItemActive('/orgs/acme/workspaces/30/users', item)).toBe(false)
  })

  it('uses route params to keep repeated navigation entries distinct', () => {
    expect(navItemKey(item)).not.toBe(
      navItemKey({
        ...item,
        params: { org_slug: 'acme', workspace_id: '4' },
      }),
    )
  })
})
