import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/orgs/$org_slug/')({
  component: OrganizationIndexRedirectPage,
})

function OrganizationIndexRedirectPage() {
  const { org_slug: orgSlug } = Route.useParams()
  return <Navigate to="/orgs/$org_slug/workspaces" params={{ org_slug: orgSlug }} replace />
}
