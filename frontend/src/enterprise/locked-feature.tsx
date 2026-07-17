// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

import { Badge } from '#/components/ui/badge'
import type { OptionalFeature } from '#/lib/product/optional-features'

export function EnterpriseLockedFeature({ feature }: { feature: OptionalFeature }) {
  return (
    <div className="mx-auto w-full max-w-lg p-6">
      <div className="flex items-center gap-2">
        <h1 className="text-lg font-semibold">{feature.title}</h1>
        <Badge variant="secondary">{feature.badge}</Badge>
      </div>
      <p className="mt-2 text-muted-foreground">{feature.description}</p>
      <p className="mt-4 text-muted-foreground">
        This installation includes the feature without an active entitlement. Apply a license key to
        enable it.
      </p>
    </div>
  )
}
