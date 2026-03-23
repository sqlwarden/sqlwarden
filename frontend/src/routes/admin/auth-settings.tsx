import { createFileRoute } from '@tanstack/react-router'
import { useInstanceSettings } from '#/lib/queries/useAdmin'
import { Badge } from '#/components/ui/badge'

export const Route = createFileRoute('/admin/auth-settings')({ component: AdminAuthSettings })

function AdminAuthSettings() {
  const { data: settings } = useInstanceSettings()
  return (
    <div className="p-8 max-w-2xl mx-auto">
      <h1 className="text-2xl font-semibold text-zinc-100 mb-6">Auth Settings</h1>
      <div className="border border-zinc-800 rounded-lg divide-y divide-zinc-800">
        <SettingRow
          label="Email / Password"
          description="Allow users to sign in with email and password"
          badge={settings?.auth_method === 'password' ? 'Enabled' : 'Disabled'}
          badgeVariant={settings?.auth_method === 'password' ? 'default' : 'secondary'}
        />
        <SettingRow label="SSO / Identity Provider" description="Configure SAML or OIDC" badge="Coming soon" badgeVariant="secondary" />
      </div>
    </div>
  )
}

function SettingRow({ label, description, badge, badgeVariant }: { label: string; description: string; badge: string; badgeVariant: 'default' | 'secondary' }) {
  return (
    <div className="px-6 py-5 flex items-center justify-between">
      <div>
        <p className="font-medium text-zinc-100">{label}</p>
        <p className="text-sm text-zinc-400 mt-1">{description}</p>
      </div>
      <Badge variant={badgeVariant}>{badge}</Badge>
    </div>
  )
}
