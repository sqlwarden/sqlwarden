import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { instanceSettingsQueryOptions, queryKeys } from '#/lib/api/query'
import type { InstanceSettings } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Textarea } from '#/components/ui/textarea'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/settings/instance')({
  component: SettingsInstancePage,
  pendingComponent: RoutePending,
})

type InstanceSettingsForm = {
  instance_name: string
  instance_description: string
  support_email: string
  public_url: string
  personal_spaces_enabled: boolean
}

type InstanceSettingsErrors = Partial<Record<keyof InstanceSettingsForm, string>>

function SettingsInstancePage() {
  const queryClient = useQueryClient()
  const settings = useQuery(instanceSettingsQueryOptions())
  const [form, setForm] = useState<InstanceSettingsForm>({
    instance_name: '',
    instance_description: '',
    support_email: '',
    public_url: '',
    personal_spaces_enabled: true,
  })
  const [fieldErrors, setFieldErrors] = useState<InstanceSettingsErrors>({})

  useEffect(() => {
    if (!settings.data) return
    setForm({
      instance_name: settings.data.instance_name,
      instance_description: settings.data.instance_description,
      support_email: settings.data.support_email,
      public_url: settings.data.public_url,
      personal_spaces_enabled: settings.data.personal_spaces_enabled,
    })
  }, [settings.data])

  useEffect(() => {
    if (!settings.error) return
    toast.error(settings.error instanceof Error ? settings.error.message : 'Failed to load instance settings')
  }, [settings.error])

  const updateSettings = useMutation({
    mutationFn: async () => api.patch<InstanceSettings>('/api/v1/instance/settings', form),
    onSuccess: async (updated) => {
      setFieldErrors({})
      setForm({
        instance_name: updated.instance_name,
        instance_description: updated.instance_description,
        support_email: updated.support_email,
        public_url: updated.public_url,
        personal_spaces_enabled: updated.personal_spaces_enabled,
      })
      toast.success('Instance settings updated')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.instanceSettings() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors({
          instance_name: error.fieldErrors.instance_name,
          instance_description: error.fieldErrors.instance_description,
          support_email: error.fieldErrors.support_email,
          public_url: error.fieldErrors.public_url,
          personal_spaces_enabled: error.fieldErrors.personal_spaces_enabled,
        })
        return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to update instance settings')
    },
  })

  if (settings.isLoading) {
    return <RoutePending />
  }

  if (settings.isError || !settings.data) {
    return (
      <div className="flex flex-col gap-1.5">
        <h2 className="text-2xl font-semibold tracking-tight">Instance</h2>
        <p className="text-muted-foreground">Failed to load instance settings.</p>
      </div>
    )
  }

  const hasChanges = hasFormChanges(form, settings.data)

  function updateField<K extends keyof InstanceSettingsForm>(field: K, value: InstanceSettingsForm[K]) {
    setForm((current) => ({ ...current, [field]: value }))
    setFieldErrors((current) => ({ ...current, [field]: undefined }))
  }

  function submitSettings(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void updateSettings.mutateAsync().catch(() => { })
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h2 className="text-2xl font-semibold tracking-tight">Instance</h2>
        <p className="text-sm text-muted-foreground">Manage instance-wide settings.</p>
      </div>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>General</CardTitle>
          <CardDescription>Basic details for this SQLWarden instance.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submitSettings}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Instance name" error={fieldErrors.instance_name}>
                <Input
                  value={form.instance_name}
                  disabled={updateSettings.isPending}
                  onChange={(event) => updateField('instance_name', event.target.value)}
                />
              </Field>
              <Field label="Public URL" error={fieldErrors.public_url}>
                <Input
                  value={form.public_url}
                  inputMode="url"
                  placeholder="https://sqlwarden.example.com"
                  disabled={updateSettings.isPending}
                  onChange={(event) => updateField('public_url', event.target.value)}
                />
              </Field>
            </div>

            <Field label="Support email" error={fieldErrors.support_email}>
              <Input
                value={form.support_email}
                type="email"
                placeholder="support@example.com"
                disabled={updateSettings.isPending}
                onChange={(event) => updateField('support_email', event.target.value)}
              />
            </Field>

            <Field label="Description" error={fieldErrors.instance_description}>
              <Textarea
                value={form.instance_description}
                rows={4}
                placeholder="Optional note shown to administrators."
                disabled={updateSettings.isPending}
                onChange={(event) => updateField('instance_description', event.target.value)}
              />
            </Field>

            <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-4">
              <Checkbox
                checked={form.personal_spaces_enabled}
                disabled={updateSettings.isPending}
                onCheckedChange={(checked) => updateField('personal_spaces_enabled', checked === true)}
              />
              <span className="flex flex-col gap-1">
                <span className="font-medium text-foreground">Enable personal spaces</span>
                <span className="text-muted-foreground">
                  Allow users to create personal workspaces outside organization RBAC. Disabling this drops active personal connection sessions.
                </span>
              </span>
            </label>

            <div className="flex justify-end">
              <Button type="submit" disabled={updateSettings.isPending || !hasChanges}>
                {updateSettings.isPending ? 'Saving...' : 'Save settings'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

    </div>
  )
}

function hasFormChanges(form: InstanceSettingsForm, settings: InstanceSettings) {
  return (
    form.instance_name !== settings.instance_name ||
    form.instance_description !== settings.instance_description ||
    form.support_email !== settings.support_email ||
    form.public_url !== settings.public_url ||
    form.personal_spaces_enabled !== settings.personal_spaces_enabled
  )
}

function Field({
  children,
  error,
  label,
}: {
  children: React.ReactNode
  error?: string
  label: string
}) {
  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
