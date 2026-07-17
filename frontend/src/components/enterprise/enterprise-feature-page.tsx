import { enterpriseModule } from '@enterprise'
import type { EnterprisePageKey } from '#/lib/enterprise/module'
import { useFeature } from '#/hooks/use-edition'
import { EnterpriseUpsell } from '#/components/enterprise/enterprise-upsell'

interface EnterpriseFeaturePageProps {
  pageKey: EnterprisePageKey
  feature: string
  title: string
  description: string
}

// Enterprise feature routes are edition-stable: the route always exists.
// The real page renders only when the build includes it AND the server
// licenses the feature — an unlicensed enterprise server shows the upsell
// with the apply-key prompt, mirroring the backend, which would reject the
// feature's API calls anyway.
export function EnterpriseFeaturePage({
  pageKey,
  feature,
  title,
  description,
}: EnterpriseFeaturePageProps) {
  const Page = enterpriseModule.pages[pageKey]
  const state = useFeature(feature)

  if (Page && state === 'active') {
    return <Page />
  }

  return <EnterpriseUpsell title={title} description={description} />
}
