// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise License. See enterprise/LICENSE.

export const ENTERPRISE_BUNDLE_MARKER = 'sqlwarden-enterprise-bundle'

export function EnterpriseOverviewPage() {
  return (
    <div className="p-6" data-bundle={ENTERPRISE_BUNDLE_MARKER}>
      <h1 className="text-lg font-semibold">Enterprise</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Enterprise module loaded. Feature surfaces arrive in later releases.
      </p>
    </div>
  )
}
