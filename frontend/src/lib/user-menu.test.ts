import { describe, expect, it } from 'vitest'
import type { SessionResponse } from '#/lib/api/types'
import { buildUserMenuItems, canReachLandingHub } from './user-menu'

function makeSession(overrides: Partial<SessionResponse> = {}): SessionResponse {
  return {
    account: { id: 1, email: 'user@example.com', name: 'User', is_active: true },
    organizations: [{ id: 1, slug: 'acme', name: 'Acme', created_at: '', updated_at: '' }],
    is_instance_admin: false,
    personal_spaces_enabled: false,
    ...overrides,
  }
}

const twoOrgs = [
  { id: 1, slug: 'acme', name: 'Acme', created_at: '', updated_at: '' },
  { id: 2, slug: 'globex', name: 'Globex', created_at: '', updated_at: '' },
]

function ids(items: ReturnType<typeof buildUserMenuItems>) {
  return items.map((item) => item.id)
}

describe('canReachLandingHub', () => {
  it('is false for a single-org session without personal spaces', () => {
    expect(canReachLandingHub(makeSession())).toBe(false)
  })

  it('is true with multiple organizations', () => {
    expect(canReachLandingHub(makeSession({ organizations: twoOrgs }))).toBe(true)
  })

  it('is true when personal spaces are enabled', () => {
    expect(canReachLandingHub(makeSession({ personal_spaces_enabled: true }))).toBe(true)
  })
})

describe('buildUserMenuItems', () => {
  it('always includes personal settings pointing at /settings/account', () => {
    const items = buildUserMenuItems({ session: makeSession() })
    const personal = items.find((item) => item.id === 'personal-settings')
    expect(personal?.to).toBe('/settings/account')
  })

  it('hides switch-organization for single-org sessions without personal spaces', () => {
    expect(ids(buildUserMenuItems({ session: makeSession() }))).not.toContain('switch-organization')
  })

  it('shows switch-organization pointing at / for multi-org sessions', () => {
    const items = buildUserMenuItems({ session: makeSession({ organizations: twoOrgs }) })
    const switcher = items.find((item) => item.id === 'switch-organization')
    expect(switcher?.to).toBe('/')
  })

  it('shows switch-organization when personal spaces are enabled', () => {
    const items = buildUserMenuItems({ session: makeSession({ personal_spaces_enabled: true }) })
    expect(ids(items)).toContain('switch-organization')
  })

  it('includes administration only for instance admins', () => {
    expect(ids(buildUserMenuItems({ session: makeSession() }))).not.toContain('administration')
    const items = buildUserMenuItems({ session: makeSession({ is_instance_admin: true }) })
    const admin = items.find((item) => item.id === 'administration')
    expect(admin?.to).toBe('/administration')
  })

  it('includes org settings first, only when orgSlug and permission are present', () => {
    expect(ids(buildUserMenuItems({ session: makeSession(), orgSlug: 'acme' }))).not.toContain(
      'org-settings',
    )
    expect(
      ids(buildUserMenuItems({ session: makeSession(), canAccessOrgSettings: true })),
    ).not.toContain('org-settings')
    const items = buildUserMenuItems({
      session: makeSession(),
      orgSlug: 'acme',
      canAccessOrgSettings: true,
    })
    expect(items[0]).toMatchObject({
      id: 'org-settings',
      to: '/orgs/$org_slug',
      params: { org_slug: 'acme' },
    })
  })
})
