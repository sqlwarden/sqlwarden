import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/account')({
  component: SettingsAccountPage,
})

function SettingsAccountPage() {
  return <div className="text-sm text-muted-foreground">Account works!</div>
}
