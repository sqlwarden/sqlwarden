import { createFileRoute } from '@tanstack/react-router'
import { EnterpriseFeaturePage } from '#/components/enterprise/enterprise-feature-page'

export const Route = createFileRoute('/administration/enterprise')({
  component: EnterpriseAdministrationPage,
})

function EnterpriseAdministrationPage() {
  return (
    <EnterpriseFeaturePage
      pageKey="enterprise-overview"
      feature="enterprise"
      title="Enterprise"
      description="Audit logging, SSO (SAML/OIDC/LDAP), SCIM provisioning, and SIEM streaming are part of SQLWarden Enterprise."
    />
  )
}
