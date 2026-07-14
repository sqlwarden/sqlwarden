import type {
  Account,
  Organization,
  Paginated,
  SessionResponse,
  SetupStatusResponse,
} from '#/lib/api/types'

const now = '2026-01-01T00:00:00Z'

export function accountFixture(overrides: Partial<Account> = {}): Account {
  return {
    id: 1,
    email: 'alex@example.com',
    name: 'Alex Ward',
    is_active: true,
    created_at: now,
    updated_at: now,
    ...overrides,
  }
}

export function organizationFixture(overrides: Partial<Organization> = {}): Organization {
  return {
    id: 1,
    slug: 'acme-cloud',
    name: 'Acme Cloud',
    member_count: 3,
    team_count: 1,
    created_at: now,
    updated_at: now,
    ...overrides,
  }
}

export function sessionFixture(overrides: Partial<SessionResponse> = {}): SessionResponse {
  return {
    account: accountFixture(),
    organizations: [organizationFixture()],
    is_instance_admin: false,
    personal_spaces_enabled: false,
    ...overrides,
  }
}

export function setupStatusFixture(
  overrides: Partial<SetupStatusResponse> = {},
): SetupStatusResponse {
  return {
    configured: true,
    access_mode: 'multi_user',
    ...overrides,
  }
}

export function paginatedFixture<T>(
  items: T[],
  overrides: Partial<Paginated<T>> = {},
): Paginated<T> {
  return {
    items,
    page: 1,
    page_size: 20,
    total: items.length,
    ...overrides,
  }
}
