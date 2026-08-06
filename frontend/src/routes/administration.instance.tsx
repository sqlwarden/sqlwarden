import { errorMessage, isApiError } from '#/lib/api/errors'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import {
  instanceConfigurationQueryOptions,
  instanceSettingsQueryOptions,
  queryKeys,
} from '#/lib/api/query'
import type { InstanceSettings } from '#/lib/api/types'
import {
  bytesInUnit,
  bytesToSize,
  byteUnitOptions,
  durationToSeconds,
  durationUnitOptions,
  secondsInUnit,
  secondsToDuration,
  sizeToBytes,
  type ByteUnit,
  type DurationUnit,
} from '#/lib/units'
import { Button } from '#/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import { Input } from '#/components/ui/input'
import { Field as FieldRoot, FieldError, FieldLabel } from '#/components/ui/field'
import { Textarea } from '#/components/ui/textarea'
import { RoutePending } from '#/components/RoutePending'
import { UnitInputField } from '#/components/settings/UnitInputField'

export const Route = createFileRoute('/administration/instance')({
  component: SettingsInstancePage,
  pendingComponent: RoutePending,
})

type InstanceSettingsForm = InstanceSettings

type InstanceSettingsErrors = Partial<Record<keyof InstanceSettingsForm, string>>

interface FormUnits {
  jwtAccessTokenTtl: DurationUnit
  queryMaxResultBytes: ByteUnit
  exportsSyncMaxBytes: ByteUnit
  exportsBackgroundMaxBytes: ByteUnit
  schemaSnapshotFreshness: DurationUnit
}

const emptyForm: InstanceSettingsForm = {
  instance_name: '',
  instance_description: '',
  support_email: '',
  public_url: '',
  personal_spaces_enabled: true,
  jwt_access_token_ttl_seconds: 3_600,
  sessions_revocation_enabled: false,
  query_max_result_rows: 1_000,
  query_max_result_bytes: 10_485_760,
  exports_sync_max_bytes: 52_428_800,
  exports_background_max_bytes: 0,
  schema_snapshot_freshness_seconds: 3_600,
  file_revisions_enabled: false,
  file_revisions_keep_latest: 10,
  error_notification_email: '',
}

function unitsFromSettings(settings: InstanceSettings): FormUnits {
  return {
    jwtAccessTokenTtl: secondsToDuration(settings.jwt_access_token_ttl_seconds).unit,
    queryMaxResultBytes: bytesToSize(settings.query_max_result_bytes).unit,
    exportsSyncMaxBytes: bytesToSize(settings.exports_sync_max_bytes).unit,
    exportsBackgroundMaxBytes: bytesToSize(settings.exports_background_max_bytes).unit,
    schemaSnapshotFreshness: secondsToDuration(settings.schema_snapshot_freshness_seconds).unit,
  }
}

function SettingsInstancePage() {
  const queryClient = useQueryClient()
  const settings = useQuery(instanceSettingsQueryOptions())
  const configuration = useQuery(instanceConfigurationQueryOptions())
  const [form, setForm] = useState<InstanceSettingsForm>(emptyForm)
  const [units, setUnits] = useState<FormUnits>(unitsFromSettings(emptyForm))
  const [fieldErrors, setFieldErrors] = useState<InstanceSettingsErrors>({})

  useEffect(() => {
    if (!settings.data) return
    setForm(settings.data)
    setUnits(unitsFromSettings(settings.data))
  }, [settings.data])

  useEffect(() => {
    if (!settings.error) return
    if (isApiError(settings.error) && settings.error.code === 'settings_unavailable') return
    toast.error(errorMessage(settings.error, 'Failed to load instance settings'))
  }, [settings.error])

  const updateSettings = useMutation({
    mutationFn: async () => api.patch<InstanceSettings>('/api/v1/instance/settings', form),
    onSuccess: async (updated) => {
      setFieldErrors({})
      setForm(updated)
      setUnits(unitsFromSettings(updated))
      toast.success('Instance settings updated')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.instanceSettings() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors(error.fieldErrors as InstanceSettingsErrors)
        return
      }
      toast.error(errorMessage(error, 'Failed to update instance settings'))
    },
  })

  if (settings.isLoading) {
    return <RoutePending />
  }

  if (isApiError(settings.error) && settings.error.code === 'settings_unavailable') {
    return (
      <div className="flex flex-col gap-4">
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Instance</h2>
        <Alert variant="destructive">
          <AlertTitle>Settings unavailable</AlertTitle>
          <AlertDescription>
            Runtime settings are temporarily unavailable. Try again shortly.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  if (settings.isError || !settings.data) {
    return (
      <div className="flex flex-col gap-1.5">
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Instance</h2>
        <p className="text-muted-foreground">Failed to load instance settings.</p>
      </div>
    )
  }

  const hasChanges = hasFormChanges(form, settings.data)
  const fileStorageMode = configuration.data?.file_storage_mode
  const fileRevisionsUnsupported =
    configuration.isLoading || configuration.isError || fileStorageMode === 'file'

  function updateField<K extends keyof InstanceSettingsForm>(
    field: K,
    value: InstanceSettingsForm[K],
  ) {
    setForm((current) => ({ ...current, [field]: value }))
    setFieldErrors((current) => ({ ...current, [field]: undefined }))
  }

  function submitSettings(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void updateSettings.mutateAsync().catch(() => {})
  }

  const disabled = updateSettings.isPending

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Instance</h2>
        <p className="text-sm text-muted-foreground">Manage instance-wide settings.</p>
      </div>

      <form className="flex flex-col gap-8" onSubmit={submitSettings}>
        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>General</CardTitle>
            <CardDescription>Basic details for this SQLWarden instance.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Instance name" error={fieldErrors.instance_name}>
                  <Input
                    aria-label="Instance name"
                    aria-invalid={Boolean(fieldErrors.instance_name) || undefined}
                    value={form.instance_name}
                    disabled={disabled}
                    onChange={(event) => updateField('instance_name', event.target.value)}
                  />
                </Field>
                <Field label="Public URL" error={fieldErrors.public_url}>
                  <Input
                    aria-label="Public URL"
                    aria-invalid={Boolean(fieldErrors.public_url) || undefined}
                    value={form.public_url}
                    inputMode="url"
                    placeholder="https://sqlwarden.example.com"
                    disabled={disabled}
                    onChange={(event) => updateField('public_url', event.target.value)}
                  />
                </Field>
              </div>

              <Field label="Support email" error={fieldErrors.support_email}>
                <Input
                  aria-label="Support email"
                  aria-invalid={Boolean(fieldErrors.support_email) || undefined}
                  value={form.support_email}
                  type="email"
                  placeholder="support@example.com"
                  disabled={disabled}
                  onChange={(event) => updateField('support_email', event.target.value)}
                />
              </Field>

              <Field label="Description" error={fieldErrors.instance_description}>
                <Textarea
                  aria-label="Description"
                  aria-invalid={Boolean(fieldErrors.instance_description) || undefined}
                  value={form.instance_description}
                  rows={4}
                  placeholder="Optional note shown to administrators."
                  disabled={disabled}
                  onChange={(event) => updateField('instance_description', event.target.value)}
                />
              </Field>

              <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-4">
                <Checkbox
                  checked={form.personal_spaces_enabled}
                  disabled={disabled}
                  onCheckedChange={(checked) =>
                    updateField('personal_spaces_enabled', checked === true)
                  }
                />
                <span className="flex flex-col gap-1">
                  <span className="font-medium text-foreground">Enable personal spaces</span>
                  <span className="text-muted-foreground">
                    Allow users to create personal workspaces outside organization RBAC. Disabling
                    this drops active personal connection sessions.
                  </span>
                </span>
              </label>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Authentication &amp; Sessions</CardTitle>
            <CardDescription>Control access token lifetime and session revocation.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-5">
              <Field label="Access token lifetime" error={fieldErrors.jwt_access_token_ttl_seconds}>
                <UnitInputField
                  label="Access token lifetime"
                  error={Boolean(fieldErrors.jwt_access_token_ttl_seconds)}
                  amount={secondsInUnit(form.jwt_access_token_ttl_seconds, units.jwtAccessTokenTtl)}
                  unit={units.jwtAccessTokenTtl}
                  options={durationUnitOptions}
                  disabled={disabled}
                  min={secondsInUnit(1, units.jwtAccessTokenTtl)}
                  onAmountChange={(amount) =>
                    updateField(
                      'jwt_access_token_ttl_seconds',
                      durationToSeconds(amount, units.jwtAccessTokenTtl),
                    )
                  }
                  onUnitChange={(unit) =>
                    setUnits((current) => ({ ...current, jwtAccessTokenTtl: unit }))
                  }
                />
              </Field>

              <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-4">
                <Checkbox
                  checked={form.sessions_revocation_enabled}
                  disabled={disabled}
                  onCheckedChange={(checked) =>
                    updateField('sessions_revocation_enabled', checked === true)
                  }
                />
                <span className="flex flex-col gap-1">
                  <span className="font-medium text-foreground">Track sessions for revocation</span>
                  <span className="text-muted-foreground">
                    Persist auth and org access sessions in the database so they can be revoked
                    before their access token expires.
                  </span>
                </span>
              </label>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Query &amp; Export Limits</CardTitle>
            <CardDescription>
              Bound result sizes and export sizes. Organizations can only tighten these limits.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Query row limit" error={fieldErrors.query_max_result_rows}>
                  <Input
                    aria-label="Query row limit"
                    aria-invalid={Boolean(fieldErrors.query_max_result_rows) || undefined}
                    type="number"
                    min={1}
                    step={1}
                    value={form.query_max_result_rows}
                    disabled={disabled}
                    onChange={(event) => {
                      const next = event.target.valueAsNumber
                      updateField('query_max_result_rows', Number.isFinite(next) ? next : 0)
                    }}
                  />
                </Field>
                <Field label="Query byte limit" error={fieldErrors.query_max_result_bytes}>
                  <UnitInputField
                    label="Query byte limit"
                    error={Boolean(fieldErrors.query_max_result_bytes)}
                    amount={bytesInUnit(form.query_max_result_bytes, units.queryMaxResultBytes)}
                    unit={units.queryMaxResultBytes}
                    options={byteUnitOptions}
                    disabled={disabled}
                    min={bytesInUnit(1, units.queryMaxResultBytes)}
                    onAmountChange={(amount) =>
                      updateField(
                        'query_max_result_bytes',
                        sizeToBytes(amount, units.queryMaxResultBytes),
                      )
                    }
                    onUnitChange={(unit) =>
                      setUnits((current) => ({ ...current, queryMaxResultBytes: unit }))
                    }
                  />
                </Field>
              </div>

              <Field label="Synchronous export limit" error={fieldErrors.exports_sync_max_bytes}>
                <UnitInputField
                  label="Synchronous export limit"
                  error={Boolean(fieldErrors.exports_sync_max_bytes)}
                  amount={bytesInUnit(form.exports_sync_max_bytes, units.exportsSyncMaxBytes)}
                  unit={units.exportsSyncMaxBytes}
                  options={byteUnitOptions}
                  disabled={disabled}
                  min={bytesInUnit(1, units.exportsSyncMaxBytes)}
                  onAmountChange={(amount) =>
                    updateField(
                      'exports_sync_max_bytes',
                      sizeToBytes(amount, units.exportsSyncMaxBytes),
                    )
                  }
                  onUnitChange={(unit) =>
                    setUnits((current) => ({ ...current, exportsSyncMaxBytes: unit }))
                  }
                />
              </Field>

              <Field
                label="Background export limit"
                error={fieldErrors.exports_background_max_bytes}
              >
                <UnitInputField
                  label="Background export limit"
                  error={Boolean(fieldErrors.exports_background_max_bytes)}
                  amount={bytesInUnit(
                    form.exports_background_max_bytes,
                    units.exportsBackgroundMaxBytes,
                  )}
                  unit={units.exportsBackgroundMaxBytes}
                  options={byteUnitOptions}
                  disabled={disabled}
                  min={0}
                  onAmountChange={(amount) =>
                    updateField(
                      'exports_background_max_bytes',
                      sizeToBytes(amount, units.exportsBackgroundMaxBytes),
                    )
                  }
                  onUnitChange={(unit) =>
                    setUnits((current) => ({ ...current, exportsBackgroundMaxBytes: unit }))
                  }
                />
                <p className="text-xs text-muted-foreground">Set to 0 for no limit.</p>
              </Field>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Schema Snapshots</CardTitle>
            <CardDescription>How often persisted schema snapshots are refreshed.</CardDescription>
          </CardHeader>
          <CardContent>
            <Field label="Snapshot freshness" error={fieldErrors.schema_snapshot_freshness_seconds}>
              <UnitInputField
                label="Snapshot freshness"
                error={Boolean(fieldErrors.schema_snapshot_freshness_seconds)}
                amount={secondsInUnit(
                  form.schema_snapshot_freshness_seconds,
                  units.schemaSnapshotFreshness,
                )}
                unit={units.schemaSnapshotFreshness}
                options={durationUnitOptions}
                disabled={disabled}
                min={secondsInUnit(1, units.schemaSnapshotFreshness)}
                onAmountChange={(amount) =>
                  updateField(
                    'schema_snapshot_freshness_seconds',
                    durationToSeconds(amount, units.schemaSnapshotFreshness),
                  )
                }
                onUnitChange={(unit) =>
                  setUnits((current) => ({ ...current, schemaSnapshotFreshness: unit }))
                }
              />
              <p className="text-xs text-muted-foreground">
                Snapshots older than this are treated as stale and refreshed on next access.
              </p>
            </Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>File Revisions</CardTitle>
            <CardDescription>Keep prior revisions of saved workspace files.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-5">
              <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-4 has-disabled:cursor-not-allowed has-disabled:opacity-50">
                <Checkbox
                  checked={form.file_revisions_enabled}
                  disabled={disabled || fileRevisionsUnsupported}
                  onCheckedChange={(checked) =>
                    updateField('file_revisions_enabled', checked === true)
                  }
                />
                <span className="flex flex-col gap-1">
                  <span className="font-medium text-foreground">Enable file revisions</span>
                  <span className="text-muted-foreground">
                    {configuration.isLoading
                      ? 'Checking whether the current file storage mode supports revisions.'
                      : configuration.isError
                        ? 'Unavailable until deployment configuration can be verified.'
                        : fileRevisionsUnsupported
                          ? 'Not available with the current file storage mode.'
                          : 'Organizations can further restrict this but cannot enable it if disabled here.'}
                  </span>
                </span>
              </label>

              {fieldErrors.file_revisions_enabled ? (
                <p className="text-xs text-destructive">{fieldErrors.file_revisions_enabled}</p>
              ) : null}

              <Field label="Revisions to keep" error={fieldErrors.file_revisions_keep_latest}>
                <Input
                  aria-label="Revisions to keep"
                  aria-invalid={Boolean(fieldErrors.file_revisions_keep_latest) || undefined}
                  type="number"
                  min={0}
                  step={1}
                  value={form.file_revisions_keep_latest}
                  disabled={disabled || fileRevisionsUnsupported || !form.file_revisions_enabled}
                  onChange={(event) => {
                    const next = event.target.valueAsNumber
                    updateField('file_revisions_keep_latest', Number.isFinite(next) ? next : 0)
                  }}
                />
              </Field>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Error Notifications</CardTitle>
            <CardDescription>Where background job failure alerts are sent.</CardDescription>
          </CardHeader>
          <CardContent>
            <Field label="Notification email" error={fieldErrors.error_notification_email}>
              <Input
                aria-label="Notification email"
                aria-invalid={Boolean(fieldErrors.error_notification_email) || undefined}
                value={form.error_notification_email}
                type="email"
                placeholder="alerts@example.com"
                disabled={disabled}
                onChange={(event) => updateField('error_notification_email', event.target.value)}
              />
            </Field>
          </CardContent>
        </Card>

        <div className="flex justify-end">
          <Button type="submit" disabled={disabled || !hasChanges}>
            {updateSettings.isPending ? 'Saving...' : 'Save settings'}
          </Button>
        </div>
      </form>
    </div>
  )
}

function hasFormChanges(form: InstanceSettingsForm, settings: InstanceSettings) {
  return (Object.keys(form) as (keyof InstanceSettingsForm)[]).some(
    (key) => form[key] !== settings[key],
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
    <FieldRoot data-invalid={Boolean(error)}>
      <FieldLabel>{label}</FieldLabel>
      {children}
      <FieldError>{error}</FieldError>
    </FieldRoot>
  )
}
