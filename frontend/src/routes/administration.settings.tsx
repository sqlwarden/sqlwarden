import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/administration/settings')({
  component: AdministrationSettingsPage,
})

function AdministrationSettingsPage() {
  return <div className="text-sm text-muted-foreground">Settings works!</div>
}
