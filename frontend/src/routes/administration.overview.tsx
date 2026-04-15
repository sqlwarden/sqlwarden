import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/overview')({
  component: AdministrationOverviewPage,
})

function AdministrationOverviewPage() {
  return <div className="text-sm text-muted-foreground">Overview works!</div>
}
