import { createFileRoute } from '@tanstack/react-router'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/settings/account')({
  component: SettingsAccountPage,
  pendingComponent: RoutePending,
})

function SettingsAccountPage() {
  return <div className="text-sm text-muted-foreground">Account works!</div>
}
