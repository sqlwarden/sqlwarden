import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/configuration')({
  component: LegacyConfigurationRedirect,
})

function LegacyConfigurationRedirect() {
  return <Navigate to="/administration/instance" replace />
}
