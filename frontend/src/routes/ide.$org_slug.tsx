import { useEffect, useState } from 'react'
import { Navigate, createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  WorkspaceEmptyState,
  WorkspaceIdeSkeleton,
  WorkspaceLoadError,
} from '#/components/ide/WorkspaceIde'
import { peekLastActiveWorkspaceId } from '#/components/ide/useIdeStore'
import { orgWorkspacesQueryOptions } from '#/lib/api/query'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { getAccessToken } from '#/lib/auth/access-token'
import { NavigateToLogin } from '#/components/auth/NavigateToLogin'

export const Route = createFileRoute('/ide/$org_slug')({
  component: IdeOrgRedirect,
  pendingComponent: WorkspaceIdeSkeleton,
})

/** Workspace-less entry point (e.g. "Open Editor" from the org sidebar).
 *  Resolves the workspace to redirect into and hands off to the real,
 *  workspace-scoped IDE route — the IDE itself never renders here. */
function IdeOrgRedirect() {
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
    return <NavigateToLogin />
  }
  return (
    <ResolveDefaultWorkspace
      orgSlug={orgSlug}
      accountId={session.data.account.id}
      nativeShell={setupStatus.data?.capabilities.native_shell === true}
    />
  )
}

function ResolveDefaultWorkspace({
  orgSlug,
  accountId,
  nativeShell,
}: {
  orgSlug: string
  accountId: number
  nativeShell: boolean
}) {
  const workspaces = useQuery(
    orgWorkspacesQueryOptions(orgSlug, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const lastActiveId = useLastActiveWorkspaceId(orgSlug, accountId)

  if (workspaces.isLoading || lastActiveId === 'loading') {
    return <WorkspaceIdeSkeleton />
  }
  if (workspaces.isError) {
    return (
      <WorkspaceLoadError
        isRetrying={workspaces.isFetching}
        onRetry={() => {
          void workspaces.refetch()
        }}
      />
    )
  }
  const items = workspaces.data?.items ?? []
  if (items.length === 0) {
    return <WorkspaceEmptyState orgSlug={orgSlug} nativeShell={nativeShell} />
  }

  const target = items.find((workspace) => workspace.id === lastActiveId) ?? items[0]
  return (
    <Navigate
      to="/orgs/$org_slug/workspaces/$workspace_id/ide"
      params={{ org_slug: orgSlug, workspace_id: String(target.id) }}
      replace
    />
  )
}

/** One-shot IndexedDB read, not a TanStack Query cache entry — this is local
 *  device state, not server data to keep in sync. */
function useLastActiveWorkspaceId(orgSlug: string, accountId: number) {
  const [id, setId] = useState<number | undefined | 'loading'>('loading')

  useEffect(() => {
    let cancelled = false
    setId('loading')
    void peekLastActiveWorkspaceId(orgSlug, accountId).then((result) => {
      if (!cancelled) setId(result)
    })
    return () => {
      cancelled = true
    }
  }, [orgSlug, accountId])

  return id
}
