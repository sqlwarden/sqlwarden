import { createFileRoute } from '@tanstack/react-router'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/environments')({
  component: WorkspaceEnvironmentsPage,
  pendingComponent: RoutePending,
})

function WorkspaceEnvironmentsPage() {
  return <PlaceholderPage title="Environments" />
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="flex flex-col gap-2">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="text-sm text-muted-foreground">{title} works!</p>
    </div>
  )
}
