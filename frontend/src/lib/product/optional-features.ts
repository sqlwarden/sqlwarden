export interface OptionalFeature {
  id: string
  title: string
  description: string
  badge: string
  upgradeUrl: string
}

export const OPTIONAL_FEATURES = {
  platform: {
    id: 'enterprise',
    title: 'Enterprise',
    description:
      'Audit logging, SAML and LDAP SSO, SCIM provisioning, and SIEM streaming are available with SQLWarden Enterprise.',
    badge: 'Enterprise',
    upgradeUrl: 'https://sqlwarden.com/enterprise',
  },
  auditLog: {
    id: 'audit_log',
    title: 'Audit logging',
    description: 'Retain durable administrative and data-access audit records.',
    badge: 'Enterprise',
    upgradeUrl: 'https://sqlwarden.com/enterprise',
  },
  enterpriseSso: {
    id: 'enterprise_sso',
    title: 'Enterprise single sign-on',
    description: 'Connect SAML and LDAP identity providers.',
    badge: 'Enterprise',
    upgradeUrl: 'https://sqlwarden.com/enterprise',
  },
  scim: {
    id: 'scim',
    title: 'SCIM provisioning',
    description: 'Provision and deactivate accounts from an identity provider.',
    badge: 'Enterprise',
    upgradeUrl: 'https://sqlwarden.com/enterprise',
  },
  siem: {
    id: 'siem',
    title: 'SIEM streaming',
    description: 'Stream security events to external monitoring systems.',
    badge: 'Enterprise',
    upgradeUrl: 'https://sqlwarden.com/enterprise',
  },
} as const satisfies Record<string, OptionalFeature>

export type OptionalFeatureDefinition = (typeof OPTIONAL_FEATURES)[keyof typeof OPTIONAL_FEATURES]
export type OptionalFeatureId = OptionalFeatureDefinition['id']
