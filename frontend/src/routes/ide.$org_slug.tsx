import { Navigate, createFileRoute } from '@tanstack/react-router'
import { WorkspaceIde, WorkspaceIdeSkeleton } from '#/components/ide/WorkspaceIde'
import { parseIdeSearch } from '#/components/ide/ideDeepLink'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { getAccessToken } from '#/lib/auth/access-token'

export const Route = createFileRoute('/ide/$org_slug')({
  component: IdePage,
  pendingComponent: WorkspaceIdeSkeleton,
  validateSearch: parseIdeSearch,
})

function IdePage() {
  const { org_slug: orgSlug } = Route.useParams()
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return <WorkspaceIdeSkeleton />
  }
  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }
  if (!hasToken || !session.data) {
    return <Navigate to="/login" replace />
  }
  return <WorkspaceIde orgSlug={orgSlug} />
}
