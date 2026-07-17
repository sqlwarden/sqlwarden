import { Badge } from '#/components/ui/badge'
import type { OptionalFeature } from '#/lib/product/optional-features'

interface UpgradePromptProps {
  feature: OptionalFeature
}

// UpgradePrompt is public product presentation. It contains no entitlement,
// license-key, or optional-feature implementation logic.
export function UpgradePrompt({ feature }: UpgradePromptProps) {
  return (
    <div className="mx-auto w-full max-w-lg p-6">
      <div className="flex items-center gap-2">
        <h1 className="text-lg font-semibold">{feature.title}</h1>
        <Badge variant="secondary">{feature.badge}</Badge>
      </div>
      <p className="mt-2 text-muted-foreground">{feature.description}</p>
      <p className="mt-4 text-muted-foreground">This feature is not included in this build.</p>
      <a
        className="mt-4 inline-block cursor-pointer text-sm font-medium text-primary hover:underline"
        href={feature.upgradeUrl}
        rel="noreferrer"
        target="_blank"
      >
        Learn more
      </a>
    </div>
  )
}
