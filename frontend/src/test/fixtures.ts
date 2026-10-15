import type {
  Account,
  ConnectionDetail,
  ConnectionFieldSpec,
  InstanceConfiguration,
  InstanceSettings,
  Organization,
  OrganizationRuntimeSettings,
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

export function connectionFieldSpecFixture(): ConnectionFieldSpec[] {
  return [
    { key: 'host', label: 'Host', type: 'string', required: true, secret: false },
    { key: 'port', label: 'Port', type: 'int', required: true, default: '5432', secret: false },
    { key: 'database', label: 'Database', type: 'string', required: false, secret: false },
    { key: 'username', label: 'Username', type: 'string', required: true, secret: false },
    { key: 'password', label: 'Password', type: 'string', required: false, secret: true },
  ]
}

export function connectionDetailFixture(
  overrides: Partial<ConnectionDetail> = {},
): ConnectionDetail {
  return {
    id: 7,
    workspace_id: 3,
    environment_id: 4,
    name: 'Analytics',
    driver: 'postgres',
    access_mode: 'open',
    show_system_schemas: false,
    show_all_databases: false,
    created_at: now,
    updated_at: now,
    params: { host: 'db.example.test', port: '5432', database: 'analytics', username: 'reader' },
    secrets: { password: { set: true, source: 'stored', revealable: true } },
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
    ...overrides,
  }
}

export function setupStatusFixture(
  overrides: Partial<SetupStatusResponse> = {},
): SetupStatusResponse {
  return {
    configured: true,
    setup_requires_input: true,
    invitations_enabled: true,
    ...overrides,
  }
}

export function instanceSettingsFixture(
  overrides: Partial<InstanceSettings> = {},
): InstanceSettings {
  return {
    instance_name: 'SQLWarden',
    instance_description: '',
    support_email: '',
    base_url: 'https://sqlwarden.example.com',
    jwt_access_token_ttl_seconds: 3_600,
    sessions_revocation_enabled: false,
    query_max_result_rows: 1_000,
    query_max_result_bytes: 10_485_760,
    query_cursor_page_size: 200,
    exports_sync_max_bytes: 52_428_800,
    exports_background_max_bytes: 0,
    file_revisions_enabled: true,
    file_revisions_keep_latest: 10,
    query_history_mode: 'backend',
    query_history_retention_count: 500,
    query_history_retention_count_max: 5_000,
    query_favorites_mode: 'backend',
    error_notification_email: '',
    log_level: 'info',
    database_query_tracing_enabled: false,
    access_logs_enabled: false,
    jobs_worker_count: 16,
    jobs_poll_interval_seconds: 1,
    jobs_claim_lease_seconds: 300,
    jobs_completed_retention_seconds: 604_800,
    smtp_enabled: false,
    smtp_host: '',
    smtp_port: 25,
    smtp_username: '',
    smtp_password_configured: false,
    smtp_from: '',
    sqlite_local_targets_enabled: true,
    sqlite_memory_targets_enabled: false,
    ...overrides,
  }
}

export function instanceConfigurationFixture(
  overrides: Partial<InstanceConfiguration> = {},
): InstanceConfiguration {
  return {
    deployment_managed: true,
    restart_required: true,
    http_port: 8080,
    profile: 'server',
    log_format: 'json',
    database_driver: 'sqlite',
    database_automigrate: true,
    tls_enabled: false,
    file_storage_mode: 'file',
    file_storage_backend: 'local',
    ...overrides,
  }
}

export function organizationRuntimeSettingsFixture(
  overrides: Partial<OrganizationRuntimeSettings> = {},
): OrganizationRuntimeSettings {
  return {
    overrides: {
      query_max_result_rows: null,
      query_max_result_bytes: null,
      exports_sync_max_bytes: null,
      exports_background_max_bytes: null,
      file_revisions_enabled: null,
      file_revisions_keep_latest: null,
      query_history_mode: null,
      query_history_retention_count: null,
      query_favorites_mode: null,
    },
    effective: {
      query_max_result_rows: 1_000,
      query_max_result_bytes: 10_485_760,
      exports_sync_max_bytes: 52_428_800,
      exports_background_max_bytes: 0,
      file_revisions_enabled: true,
      file_revisions_keep_latest: 10,
      query_history_mode: 'backend',
      query_history_retention_count: 500,
      query_favorites_mode: 'backend',
    },
    constraints: {
      query_max_result_rows_max: 1_000,
      query_max_result_bytes_max: 10_485_760,
      exports_sync_max_bytes_max: 52_428_800,
      exports_background_max_bytes_max: 0,
      file_revisions_available: true,
      file_revisions_keep_latest_max: 10,
      query_history_retention_count_max: 5_000,
    },
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
