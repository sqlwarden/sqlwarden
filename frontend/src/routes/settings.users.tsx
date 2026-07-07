import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/users')({
  component: SettingsUsersRedirectPage,
})

function SettingsUsersRedirectPage() {
  return <Navigate to="/administration/users" replace />
}
