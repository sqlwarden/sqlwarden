import type { ReactNode } from 'react'
import { extensionModule } from '@extensions'
import type { ExtensionSlotKey } from '#/lib/extensions/module'
import type { OptionalFeature } from '#/lib/product/optional-features'
import { useCapability } from '#/hooks/use-capability'
import { Button } from '#/components/ui/button'

interface ExtensionSlotProps {
  slot: ExtensionSlotKey
  feature: OptionalFeature
  fallback?: ReactNode
}

export function ExtensionSlot({ slot, feature, fallback = null }: ExtensionSlotProps) {
  const Impl = extensionModule.slots[slot]

  if (!Impl) {
    return <>{fallback}</>
  }

  return <AvailableExtensionSlot Impl={Impl} feature={feature} fallback={fallback} />
}

function AvailableExtensionSlot({
  Impl,
  feature,
  fallback,
}: Omit<ExtensionSlotProps, 'slot'> & {
  Impl: NonNullable<(typeof extensionModule.slots)[ExtensionSlotKey]>
}) {
  const access = useCapability(feature.id)

  if (access.state === 'active') return <Impl />
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
