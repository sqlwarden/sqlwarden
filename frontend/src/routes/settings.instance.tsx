import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/instance')({
  component: SettingsInstanceRedirectPage,
})

function SettingsInstanceRedirectPage() {
  return <Navigate to="/administration/instance" replace />
}
