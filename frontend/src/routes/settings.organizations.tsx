import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/organizations')({
  component: SettingsOrganizationsRedirectPage,
})

function SettingsOrganizationsRedirectPage() {
  return <Navigate to="/administration/organizations" replace />
}
