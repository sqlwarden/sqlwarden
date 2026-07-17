// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

import type { ExtensionModule } from '#/lib/extensions/module'
import { EnterpriseLockedFeature } from './locked-feature'
import { EnterpriseOverviewPage } from './overview-page'

export const extensionModule: ExtensionModule = {
  pages: {
    'platform-overview': EnterpriseOverviewPage,
  },
  slots: {},
  LockedFeature: EnterpriseLockedFeature,
}
