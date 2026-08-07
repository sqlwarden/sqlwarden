import { createFileRoute } from '@tanstack/react-router'
import { RoutePending } from '#/components/RoutePending'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/settings/api-tokens')({
  component: SettingsApiTokensPage,
  pendingComponent: RoutePending,
})

function SettingsApiTokensPage() {
  usePageTitle('API Tokens', 'Settings')
  return <div className="text-sm text-muted-foreground">API Tokens works!</div>
}
