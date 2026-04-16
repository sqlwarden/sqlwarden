import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration')({
  component: AdministrationRedirectPage,
})

function AdministrationRedirectPage() {
  return <Navigate to="/settings/administrators" replace />
}
