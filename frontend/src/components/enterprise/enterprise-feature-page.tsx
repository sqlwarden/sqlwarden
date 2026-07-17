import { enterpriseModule } from '@enterprise'
import type { EnterprisePageKey } from '#/lib/enterprise/module'
import { useEdition } from '#/hooks/use-edition'
import { Badge } from '#/components/ui/badge'

interface EnterpriseFeaturePageProps {
  pageKey: EnterprisePageKey
  title: string
  description: string
}

// Enterprise feature routes are edition-stable: the route always exists, and
// this component resolves the real page from the enterprise module when the
// build includes it, falling back to the upsell state otherwise.
export function EnterpriseFeaturePage({ pageKey, title, description }: EnterpriseFeaturePageProps) {
  const Page = enterpriseModule.pages[pageKey]
  const edition = useEdition()

  if (Page) {
    return <Page />
  }

  const locked = edition.data?.edition === 'enterprise'

  return (
    <div className="mx-auto w-full max-w-lg p-6">
      <div className="flex items-center gap-2">
        <h1 className="text-lg font-semibold">{title}</h1>
        <Badge variant="secondary">Enterprise</Badge>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      <p className="mt-4 text-sm text-muted-foreground">
        {locked
          ? 'This server runs SQLWarden Enterprise without an active license. Apply a license key to enable this feature.'
          : 'This feature is part of SQLWarden Enterprise.'}
      </p>
    </div>
  )
}
