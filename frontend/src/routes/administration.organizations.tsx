import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/organizations')({
  component: AdministrationOrganizationsRedirectPage,
})

function AdministrationOrganizationsRedirectPage() {
  return <Navigate to="/settings/organizations" replace />
}
