import type { ReactNode } from 'react'
import { enterpriseModule } from '@enterprise'
import type { EnterpriseSlotKey } from '#/lib/enterprise/module'
import { useFeature } from '#/hooks/use-edition'

interface EnterpriseSlotProps {
  slot: EnterpriseSlotKey
  feature: string
  fallback?: ReactNode
}

// Renders an enterprise-only section inside a shared core page. The
// implementation renders only when the build registers it AND the server
// licenses the feature; otherwise the fallback (default: nothing). Use a
// fallback for locked/upsell affordances, omit it for surfaces that should
// simply not exist without a license (e.g. SSO buttons on the login page).
export function EnterpriseSlot({ slot, feature, fallback = null }: EnterpriseSlotProps) {
  const Impl = enterpriseModule.slots[slot]
  const state = useFeature(feature)

  if (Impl && state === 'active') {
    return <Impl />
  }

  return <>{fallback}</>
}
