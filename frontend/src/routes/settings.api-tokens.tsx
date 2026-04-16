import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/settings/api-tokens')({
  component: SettingsApiTokensPage,
})

function SettingsApiTokensPage() {
  return <div className="text-sm text-muted-foreground">API Tokens works!</div>
}
