import { Navigate, createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/')({
  component: AdministrationIndexPage,
})

function AdministrationIndexPage() {
  return <Navigate to="/administration/overview" replace />
}
