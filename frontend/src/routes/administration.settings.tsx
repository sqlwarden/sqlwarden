import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/settings')({
  component: AdministrationSettingsRedirectPage,
})

function AdministrationSettingsRedirectPage() {
  return <Navigate to="/settings/account" replace />
}
