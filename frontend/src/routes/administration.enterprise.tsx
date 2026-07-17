import { createFileRoute } from '@tanstack/react-router'
import { EnterpriseFeaturePage } from '#/components/enterprise/enterprise-feature-page'
import { ENTERPRISE_FEATURES } from '#/lib/enterprise/features'

export const Route = createFileRoute('/administration/enterprise')({
  component: EnterpriseAdministrationPage,
})

function EnterpriseAdministrationPage() {
  return (
    <EnterpriseFeaturePage
      pageKey="enterprise-overview"
      feature={ENTERPRISE_FEATURES.platform}
      title="Enterprise"
      description="Audit logging, SAML and LDAP SSO, SCIM provisioning, and SIEM streaming are part of SQLWarden Enterprise."
    />
  )
}
