import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/administrators')({
  component: AdministrationAdministratorsRedirectPage,
})

function AdministrationAdministratorsRedirectPage() {
  return <Navigate to="/settings/administrators" replace />
}
