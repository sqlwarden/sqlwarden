import { extensionModule } from '@extensions'
import type { ExtensionPageKey } from '#/lib/extensions/module'
import type { OptionalFeature } from '#/lib/product/optional-features'
import { useCapability } from '#/hooks/use-capability'
import { UpgradePrompt } from '#/components/extensions/upgrade-prompt'
import { Button } from '#/components/ui/button'

interface OptionalFeaturePageProps {
  pageKey: ExtensionPageKey
  feature: OptionalFeature
}

// A missing build-time implementation renders public upgrade information.
// When an implementation exists, generic capability state selects the real
// page or delegates the locked presentation back to that extension.
export function OptionalFeaturePage({ pageKey, feature }: OptionalFeaturePageProps) {
  const Page = extensionModule.pages[pageKey]

  if (!Page) {
    return <UpgradePrompt feature={feature} />
  }

  return <AvailableFeaturePage Page={Page} feature={feature} />
}

function AvailableFeaturePage({
  Page,
  feature,
}: {
  Page: NonNullable<(typeof extensionModule.pages)[ExtensionPageKey]>
  feature: OptionalFeature
}) {
  const access = useCapability(feature.id)

  if (access.state === 'active') return <Page />
  if (access.state === 'loading') {
    return <div className="p-6 text-muted-foreground">Loading…</div>
  }
  if (access.state === 'error') {
    return (
      <div className="space-y-3 p-6">
        <p className="text-destructive">Unable to load feature availability.</p>
        <Button className="cursor-pointer" variant="outline" onClick={access.retry}>
          Retry
        </Button>
      </div>
    )
  }

  const LockedFeature = extensionModule.LockedFeature
  return LockedFeature ? <LockedFeature feature={feature} /> : <UpgradePrompt feature={feature} />
}
