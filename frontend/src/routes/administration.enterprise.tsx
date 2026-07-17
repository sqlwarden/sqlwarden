import { createFileRoute } from '@tanstack/react-router'
import { OptionalFeaturePage } from '#/components/extensions/optional-feature-page'
import { OPTIONAL_FEATURES } from '#/lib/product/optional-features'

export const Route = createFileRoute('/administration/enterprise')({
  component: EnterpriseAdministrationPage,
})

function EnterpriseAdministrationPage() {
  return <OptionalFeaturePage pageKey="platform-overview" feature={OPTIONAL_FEATURES.platform} />
}
