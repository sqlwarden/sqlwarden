import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/overview')({
  component: AdministrationOverviewRedirectPage,
})

function AdministrationOverviewRedirectPage() {
  return <Navigate to="/settings/administrators" replace />
}
