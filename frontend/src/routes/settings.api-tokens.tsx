import { createFileRoute } from '@tanstack/react-router'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/settings/api-tokens')({
  component: SettingsApiTokensPage,
  pendingComponent: RoutePending,
})

function SettingsApiTokensPage() {
  return <div className="text-sm text-muted-foreground">API Tokens works!</div>
}
