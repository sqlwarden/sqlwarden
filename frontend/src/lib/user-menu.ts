import type { SessionResponse } from '#/lib/api/types'
import type { AppIcon } from '#/lib/icons'

export type UserMenuItem = {
  id: 'org-settings' | 'personal-settings' | 'switch-organization' | 'administration'
  to: string
  params?: Record<string, string>
  label: string
  icon: AppIcon
}

/** The landing hub at `/` redirects single-org sessions (without personal
 *  spaces) straight into the IDE, so it is only a meaningful destination
 *  when there is actually a choice to make. */
export function canReachLandingHub(session: SessionResponse): boolean {
  return session.personal_spaces_enabled || session.organizations.length !== 1
}

/** Single source of truth for the user menu shown on every surface
 *  (landing, admin shells, IDE). Surfaces render these items verbatim,
 *  then append their own Sign out action. */
export function buildUserMenuItems({
  session,
  orgSlug,
  canAccessOrgSettings,
}: {
  session: SessionResponse
  orgSlug?: string
  canAccessOrgSettings?: boolean
}): UserMenuItem[] {
  const items: UserMenuItem[] = []

  if (orgSlug && canAccessOrgSettings) {
    items.push({
      id: 'org-settings',
      to: '/orgs/$org_slug',
      params: { org_slug: orgSlug },
      label: 'Organization Settings',
      icon: 'settings-02',
    })
  }

  items.push({ id: 'personal-settings', to: '/settings/account', label: 'Personal Settings', icon: 'user-02' })

  if (canReachLandingHub(session)) {
    items.push({ id: 'switch-organization', to: '/', label: 'Switch Organization', icon: 'building-04' })
  }

  if (session.is_instance_admin) {
    items.push({ id: 'administration', to: '/administration', label: 'Administration', icon: 'shield-user' })
  }

  return items
}
