import { Navigate, createFileRoute } from '@tanstack/react-router'
import { useLayoutWidth } from '#/components/layout-width-provider'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { getAccessToken } from '#/lib/auth/access-token'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/orgs/$org_slug/workspaces')({
  component: OrganizationWorkspacesPage,
})

function OrganizationWorkspacesPage() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const { isExpanded } = useLayoutWidth()
  const { org_slug: orgSlug } = Route.useParams()

  if (setupStatus.isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading…</div>
      </main>
    )
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (!hasToken) {
    return <Navigate to="/login" replace />
  }

  return (
    <main
      className={cn(
        'py-12',
        isExpanded ? 'w-full px-4 sm:px-6' : 'container mx-auto max-w-6xl px-4',
      )}
    >
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">{orgSlug}</h1>
        <p className="text-sm text-muted-foreground">Organization landing page works.</p>
      </div>
    </main>
  )
}
