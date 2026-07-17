import { enterpriseModule } from '@enterprise'
import type { EnterpriseFeature } from '#/lib/enterprise/features'
import type { EnterprisePageKey } from '#/lib/enterprise/module'
import { useFeature } from '#/hooks/use-edition'
import { EnterpriseUpsell } from '#/components/enterprise/enterprise-upsell'
import { Button } from '#/components/ui/button'

interface EnterpriseFeaturePageProps {
  pageKey: EnterprisePageKey
  feature: EnterpriseFeature
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

  if (!Page) {
    return <EnterpriseUpsell title={title} description={description} enterprise={false} />
  }

  return (
    <LicensedEnterprisePage Page={Page} feature={feature} title={title} description={description} />
  )
}

function LicensedEnterprisePage({
  Page,
  feature,
  title,
  description,
}: Omit<EnterpriseFeaturePageProps, 'pageKey'> & {
  Page: NonNullable<(typeof enterpriseModule.pages)[EnterprisePageKey]>
}) {
  const access = useFeature(feature)

  if (access.state === 'active') return <Page />
  if (access.state === 'loading') {
    return <div className="p-6 text-sm text-muted-foreground">Loading…</div>
  }
  if (access.state === 'error') {
    return (
      <div className="space-y-3 p-6">
        <p className="text-sm text-destructive">Unable to load license information.</p>
        <Button className="cursor-pointer" variant="outline" onClick={access.retry}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <EnterpriseUpsell
      title={title}
      description={description}
      enterprise={access.state === 'locked'}
    />
  )
}
