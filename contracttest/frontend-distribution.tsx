import type { PropsWithChildren } from 'react'
import { Button } from '#/components/ui/button'
import type { FrontendDependencies } from '#/distribution/types'

function EnterpriseProvider({ children }: PropsWithChildren) {
  return <div className="enterprise-contract-sentinel">{children}</div>
}

function ApprovalPage() {
  return <Button>Approval queue</Button>
}

export const distribution: FrontendDependencies = {
  providers: [EnterpriseProvider],
  routes: [{ scope: 'organization', path: 'approvals', component: ApprovalPage }],
  navigation: [
    {
      scope: 'organization',
      section: 'Security',
      item: ({ orgSlug }) => ({
        to: `/orgs/${orgSlug}/approvals`,
        label: 'Approvals',
        icon: 'shield-user',
      }),
    },
  ],
}
