import { Navigate, createFileRoute } from '@tanstack/react-router'
import { WorkspaceIde, WorkspaceIdeSkeleton } from '#/components/ide/WorkspaceIde'
import { parseIdeSearch } from '#/components/ide/ideDeepLink'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { getAccessToken } from '#/lib/auth/access-token'
import { NavigateToLogin } from '#/components/auth/NavigateToLogin'

export const Route = createFileRoute('/orgs/$org_slug_/workspaces/$workspace_id/ide')({
  component: IdePage,
  pendingComponent: WorkspaceIdeSkeleton,
  validateSearch: parseIdeSearch,
})

function IdePage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
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
    return <NavigateToLogin />
  }
  return <WorkspaceIde orgSlug={orgSlug} workspaceId={Number(workspaceId)} />
}
