import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/administrators')({
  component: SettingsAdministratorsRedirectPage,
})

function SettingsAdministratorsRedirectPage() {
  return <Navigate to="/administration/administrators" replace />
}
