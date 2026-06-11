import { Navigate, createFileRoute } from '@tanstack/react-router'
import { WorkspaceIde } from '#/components/ide/WorkspaceIde'
import { RoutePending } from '#/components/RoutePending'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { getAccessToken } from '#/lib/auth/access-token'

export const Route = createFileRoute('/ide/$org_slug')({
  component: IdePage,
  pendingComponent: RoutePending,
})

function IdePage() {
  const { org_slug: orgSlug } = Route.useParams()
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading...</div>
      </main>
    )
  }
  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }
  if (!hasToken || !session.data) {
    return <Navigate to="/login" replace />
  }
  return <WorkspaceIde orgSlug={orgSlug} />
}
