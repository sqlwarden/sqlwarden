// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

import type { EnterpriseModule } from '#/lib/enterprise/module'
import { EnterpriseOverviewPage } from './overview-page'

export const enterpriseModule: EnterpriseModule = {
  pages: {
    'enterprise-overview': EnterpriseOverviewPage,
  },
  slots: {},
}
