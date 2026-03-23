import { createFileRoute } from '@tanstack/react-router'
import { useInstanceSettings, useUpdateInstanceSetting } from '#/lib/queries/useAdmin'
import { Label } from '#/components/ui/label'
import { Switch } from '#/components/ui/switch'

export const Route = createFileRoute('/admin/instance-settings')({ component: AdminInstanceSettings })

function AdminInstanceSettings() {
  const { data: settings } = useInstanceSettings()
  const updateSetting = useUpdateInstanceSetting()
  const personalOrgsEnabled = settings?.personal_orgs_enabled === 'true'

  return (
    <div className="p-8 max-w-2xl mx-auto">
      <h1 className="text-2xl font-semibold text-zinc-100 mb-6">Instance Settings</h1>
      <div className="border border-zinc-800 rounded-lg divide-y divide-zinc-800">
        <div className="px-6 py-5 flex items-center justify-between">
          <div>
            <Label className="text-base text-zinc-100">Personal Organizations</Label>
            <p className="text-sm text-zinc-400 mt-1">Allow each user to have their own personal org space</p>
          </div>
          <Switch
            checked={personalOrgsEnabled}
            onCheckedChange={(checked: boolean) =>
              updateSetting.mutate({ key: 'personal_orgs_enabled', value: String(checked) })
            }
          />
        </div>
      </div>
    </div>
  )
}
