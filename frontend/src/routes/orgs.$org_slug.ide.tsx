import { createFileRoute } from '@tanstack/react-router'
import { WorkspaceIde } from '#/components/ide/WorkspaceIde'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/orgs/$org_slug/ide')({
  component: OrganizationIdePage,
  pendingComponent: RoutePending,
})

function OrganizationIdePage() {
  const { org_slug: orgSlug } = Route.useParams()
  return <WorkspaceIde orgSlug={orgSlug} />
}
