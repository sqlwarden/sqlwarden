import { createFileRoute } from '@tanstack/react-router'
import { RoutePending } from '#/components/RoutePending'
import { usePageTitle } from '#/lib/page-title'
import { EmptyState } from '#/components/EmptyState'

export const Route = createFileRoute('/settings/api-tokens')({
  component: SettingsApiTokensPage,
  pendingComponent: RoutePending,
})

function SettingsApiTokensPage() {
  usePageTitle('API Tokens', 'Settings')
  return (
    <EmptyState
      icon="key-01"
      message="API Tokens"
      description="Personal API tokens for programmatic access aren't available yet. Coming soon."
    />
  )
}
