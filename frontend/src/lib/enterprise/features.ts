export const ENTERPRISE_FEATURES = {
  platform: 'enterprise',
  auditLog: 'audit_log',
  sso: 'sso',
  scim: 'scim',
  siem: 'siem',
} as const

export type EnterpriseFeature = (typeof ENTERPRISE_FEATURES)[keyof typeof ENTERPRISE_FEATURES]
