import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/')({
  component: AdministrationIndexRedirectPage,
})

function AdministrationIndexRedirectPage() {
  return <Navigate to="/settings/administrators" replace />
}
