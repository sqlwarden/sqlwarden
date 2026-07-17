import type { ReactNode } from 'react'
import { enterpriseModule } from '@enterprise'
import type { EnterpriseFeature } from '#/lib/enterprise/features'
import type { EnterpriseSlotKey } from '#/lib/enterprise/module'
import { useFeature } from '#/hooks/use-edition'
import { Button } from '#/components/ui/button'

interface EnterpriseSlotProps {
  slot: EnterpriseSlotKey
  feature: EnterpriseFeature
  fallback?: ReactNode
}

// Renders an enterprise-only section inside a shared core page. The
// implementation renders only when the build registers it AND the server
// licenses the feature; otherwise the fallback (default: nothing). Use a
// fallback for locked/upsell affordances. Loading and transport failures are
// rendered explicitly whenever an implementation is present in this build.
export function EnterpriseSlot({ slot, feature, fallback = null }: EnterpriseSlotProps) {
  const Impl = enterpriseModule.slots[slot]

  if (!Impl) {
    return <>{fallback}</>
  }

  return <LicensedEnterpriseSlot Impl={Impl} feature={feature} fallback={fallback} />
}

function LicensedEnterpriseSlot({
  Impl,
  feature,
  fallback,
}: Omit<EnterpriseSlotProps, 'slot'> & {
  Impl: NonNullable<(typeof enterpriseModule.slots)[EnterpriseSlotKey]>
}) {
  const access = useFeature(feature)

  if (access.state === 'active') {
    return <Impl />
  }

  if (access.state === 'loading') {
    return (
      <p className="mt-4 text-center text-sm text-muted-foreground" role="status">
        Loading additional sign-in options…
      </p>
    )
  }

  if (access.state === 'error') {
    return (
      <div className="mt-4 text-center">
        <Button className="cursor-pointer" variant="ghost" size="sm" onClick={access.retry}>
          Unable to load additional sign-in options. Retry
        </Button>
      </div>
    )
  }

  return <>{fallback}</>
}
