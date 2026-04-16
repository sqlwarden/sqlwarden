import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/profile')({
  component: SettingsProfilePage,
})

function SettingsProfilePage() {
  return <Navigate to="/account" replace />
}
