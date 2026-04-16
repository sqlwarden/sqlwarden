import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/organizations')({
  component: OrganizationsRedirectPage,
})

function OrganizationsRedirectPage() {
  return <Navigate to="/settings/my-organizations" replace />
}
