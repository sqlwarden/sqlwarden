export const ENTERPRISE_FEATURES = {
  platform: 'enterprise',
  auditLog: 'audit_log',
  enterpriseSso: 'enterprise_sso',
  scim: 'scim',
  siem: 'siem',
} as const

export type EnterpriseFeature = (typeof ENTERPRISE_FEATURES)[keyof typeof ENTERPRISE_FEATURES]
