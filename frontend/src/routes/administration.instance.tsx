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
import type {
  InstanceConfiguration,
  InstanceSettings,
  InstanceSettingsPatch,
  LogLevel,
} from '#/lib/api/types'
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
import { Badge } from '#/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import { Input } from '#/components/ui/input'
import { PasswordInput } from '#/components/ui/password-input'
import {
  Field as FieldRoot,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '#/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Textarea } from '#/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { RoutePending } from '#/components/RoutePending'
import { UnitInputField } from '#/components/settings/UnitInputField'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/administration/instance')({
  component: SettingsInstancePage,
  pendingComponent: RoutePending,
})

type InstanceSettingsForm = InstanceSettings

type InstanceSettingsErrors = Partial<Record<keyof InstanceSettingsForm | 'smtp_password', string>>

const dataSettingsFields = new Set<string>([
  'query_max_result_rows',
  'query_max_result_bytes',
  'query_cursor_page_size',
  'exports_sync_max_bytes',
  'exports_background_max_bytes',
  'schema_snapshot_freshness_seconds',
  'file_revisions_enabled',
  'file_revisions_keep_latest',
])

const operationsSettingsFields = new Set<string>([
  'error_notification_email',
  'log_level',
  'database_query_tracing_enabled',
  'access_logs_enabled',
  'jobs_worker_count',
  'jobs_poll_interval_seconds',
  'jobs_claim_lease_seconds',
  'jobs_completed_retention_seconds',
])

function sectionForFieldErrors(errors: InstanceSettingsErrors) {
  const firstField = Object.keys(errors)[0]
  if (!firstField) return 'general'
  if (dataSettingsFields.has(firstField)) return 'data'
  if (operationsSettingsFields.has(firstField)) return 'operations'
  if (firstField.startsWith('smtp_')) return 'email'
  return 'general'
}

interface FormUnits {
  jwtAccessTokenTtl: DurationUnit
  queryMaxResultBytes: ByteUnit
  exportsSyncMaxBytes: ByteUnit
  exportsBackgroundMaxBytes: ByteUnit
  schemaSnapshotFreshness: DurationUnit
  jobsPollInterval: DurationUnit
  jobsClaimLease: DurationUnit
  jobsCompletedRetention: DurationUnit
}

const emptyForm: InstanceSettingsForm = {
  instance_name: '',
  instance_description: '',
  support_email: '',
  base_url: '',
  personal_spaces_enabled: true,
  jwt_access_token_ttl_seconds: 3_600,
  sessions_revocation_enabled: false,
  query_max_result_rows: 1_000,
  query_max_result_bytes: 10_485_760,
  query_cursor_page_size: 200,
  exports_sync_max_bytes: 52_428_800,
  exports_background_max_bytes: 0,
  schema_snapshot_freshness_seconds: 3_600,
  file_revisions_enabled: false,
  file_revisions_keep_latest: 10,
  query_history_mode: 'backend',
  query_history_retention_count: 200,
  query_history_retention_count_max: 2_000,
  query_favorites_mode: 'backend',
  error_notification_email: '',
  log_level: 'info',
  database_query_tracing_enabled: false,
  access_logs_enabled: false,
  jobs_worker_count: 16,
  jobs_poll_interval_seconds: 1,
  jobs_claim_lease_seconds: 300,
  jobs_completed_retention_seconds: 604_800,
  smtp_enabled: false,
  smtp_host: '',
  smtp_port: 25,
  smtp_username: '',
  smtp_password_configured: false,
  smtp_from: '',
}

const logLevelOptions: { value: LogLevel; label: string }[] = [
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' },
]

/**
 * SMTP password field is write-only: the API never returns the secret, so the local input
 * always starts blank. `unchanged` preserves the saved secret, `set` replaces it, `clear` sends
 * null.
 */
type SmtpPasswordAction = { kind: 'unchanged' } | { kind: 'set'; value: string } | { kind: 'clear' }

function smtpPasswordInputValue(action: SmtpPasswordAction): string {
  return action.kind === 'set' ? action.value : ''
}

function smtpPasswordStatusText(action: SmtpPasswordAction, configured: boolean): string {
  if (action.kind === 'clear') return 'Password will be cleared when you save.'
  if (action.kind === 'set') return 'A new password will be saved.'
  return configured ? 'Password configured.' : 'No password configured.'
}

function buildSettingsPatch(
  form: InstanceSettingsForm,
  passwordAction: SmtpPasswordAction,
): InstanceSettingsPatch {
  const { smtp_password_configured: _smtpPasswordConfigured, ...rest } = form
  const patch: InstanceSettingsPatch = { ...rest }
  if (passwordAction.kind === 'set') {
    patch.smtp_password = passwordAction.value
  } else if (passwordAction.kind === 'clear') {
    patch.smtp_password = null
  }
  return patch
}

function unitsFromSettings(settings: InstanceSettings): FormUnits {
  return {
    jwtAccessTokenTtl: secondsToDuration(settings.jwt_access_token_ttl_seconds).unit,
    queryMaxResultBytes: bytesToSize(settings.query_max_result_bytes).unit,
    exportsSyncMaxBytes: bytesToSize(settings.exports_sync_max_bytes).unit,
    exportsBackgroundMaxBytes: bytesToSize(settings.exports_background_max_bytes).unit,
    schemaSnapshotFreshness: secondsToDuration(settings.schema_snapshot_freshness_seconds).unit,
    jobsPollInterval: secondsToDuration(settings.jobs_poll_interval_seconds).unit,
    jobsClaimLease: secondsToDuration(settings.jobs_claim_lease_seconds).unit,
    jobsCompletedRetention: secondsToDuration(settings.jobs_completed_retention_seconds).unit,
  }
}

function SettingsInstancePage() {
  usePageTitle('Settings', 'Administration')
  const queryClient = useQueryClient()
  const settings = useQuery(instanceSettingsQueryOptions())
  const configuration = useQuery(instanceConfigurationQueryOptions())
  const [form, setForm] = useState<InstanceSettingsForm>(emptyForm)
  const [units, setUnits] = useState<FormUnits>(unitsFromSettings(emptyForm))
  const [fieldErrors, setFieldErrors] = useState<InstanceSettingsErrors>({})
  const [activeSection, setActiveSection] = useState('general')
  const [smtpPasswordAction, setSmtpPasswordAction] = useState<SmtpPasswordAction>({
    kind: 'unchanged',
  })

  useEffect(() => {
    if (!settings.data) return
    setForm(settings.data)
    setUnits(unitsFromSettings(settings.data))
    setSmtpPasswordAction({ kind: 'unchanged' })
  }, [settings.data])

  useEffect(() => {
    if (!settings.error) return
    if (isApiError(settings.error) && settings.error.code === 'settings_unavailable') return
    toast.error(errorMessage(settings.error, 'Failed to load instance settings'))
  }, [settings.error])

  const updateSettings = useMutation({
    mutationFn: async () =>
      api.patch<InstanceSettings>(
        '/api/v1/instance/settings',
        buildSettingsPatch(form, smtpPasswordAction),
      ),
    onSuccess: async (updated) => {
      setFieldErrors({})
      setForm(updated)
      setUnits(unitsFromSettings(updated))
      setSmtpPasswordAction({ kind: 'unchanged' })
      toast.success('Instance settings updated')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.instanceSettings() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        const errors = error.fieldErrors as InstanceSettingsErrors
        setFieldErrors(errors)
        setActiveSection(sectionForFieldErrors(errors))
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
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Settings</h2>
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
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Settings</h2>
        <p className="text-muted-foreground">Failed to load instance settings.</p>
      </div>
    )
  }

  const hasChanges = hasFormChanges(form, settings.data) || smtpPasswordAction.kind !== 'unchanged'
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

  function updateSmtpPasswordAction(action: SmtpPasswordAction) {
    setSmtpPasswordAction(action)
    setFieldErrors((current) => ({ ...current, smtp_password: undefined }))
  }

  function submitSettings(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void updateSettings.mutateAsync().catch(() => {})
  }

  const disabled = updateSettings.isPending

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <h2 className="font-heading text-2xl font-semibold tracking-tight">Settings</h2>
        <p className="text-sm text-muted-foreground">
          Configure runtime behavior and review deployment configuration for this SQLWarden
          instance.
        </p>
      </div>

      <form className="flex flex-col gap-6" onSubmit={submitSettings}>
        <Tabs value={activeSection} onValueChange={setActiveSection} className="gap-6">
          <div className="flex flex-col gap-3 border-b border-border sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0 overflow-x-auto overflow-y-hidden">
              <TabsList variant="line" className="min-w-max">
                <TabsTrigger value="general">General</TabsTrigger>
                <TabsTrigger value="data">Data</TabsTrigger>
                <TabsTrigger value="operations">Operations</TabsTrigger>
                <TabsTrigger value="email">Email</TabsTrigger>
                <TabsTrigger value="deployment">Deployment</TabsTrigger>
              </TabsList>
            </div>

            {activeSection !== 'deployment' || hasChanges ? (
              <div
                aria-label="Settings actions"
                className="flex shrink-0 items-center justify-end gap-3 pb-2 sm:pb-1"
                role="region"
              >
                {hasChanges ? (
                  <p aria-live="polite" className="text-xs text-muted-foreground" role="status">
                    You have unsaved changes.
                  </p>
                ) : null}
                <Button type="submit" disabled={disabled || !hasChanges}>
                  {updateSettings.isPending ? 'Saving...' : 'Save settings'}
                </Button>
              </div>
            ) : null}
          </div>

          <TabsContent value="general" className="flex flex-col gap-6">
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
                    <Field label="Base URL" error={fieldErrors.base_url}>
                      <Input
                        aria-label="Base URL"
                        aria-invalid={Boolean(fieldErrors.base_url) || undefined}
                        value={form.base_url}
                        inputMode="url"
                        placeholder="https://sqlwarden.example.com"
                        required
                        disabled={disabled}
                        onChange={(event) => updateField('base_url', event.target.value)}
                      />
                      <FieldDescription>
                        Canonical URL used for authentication and generated links. Changing it can
                        interrupt active sign-in flows and requires updating external identity
                        provider callbacks.
                      </FieldDescription>
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

                  <label className="flex cursor-pointer items-start gap-3 py-2">
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
                        Allow users to create personal workspaces outside organization RBAC.
                        Disabling this drops active personal connection sessions.
                      </span>
                    </span>
                  </label>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Authentication &amp; Sessions</CardTitle>
                <CardDescription>
                  Control access token lifetime and session revocation.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col gap-5">
                  <Field
                    label="Access token lifetime"
                    error={fieldErrors.jwt_access_token_ttl_seconds}
                  >
                    <UnitInputField
                      label="Access token lifetime"
                      error={Boolean(fieldErrors.jwt_access_token_ttl_seconds)}
                      amount={secondsInUnit(
                        form.jwt_access_token_ttl_seconds,
                        units.jwtAccessTokenTtl,
                      )}
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

                  <label className="flex cursor-pointer items-start gap-3 py-2">
                    <Checkbox
                      checked={form.sessions_revocation_enabled}
                      disabled={disabled}
                      onCheckedChange={(checked) =>
                        updateField('sessions_revocation_enabled', checked === true)
                      }
                    />
                    <span className="flex flex-col gap-1">
                      <span className="font-medium text-foreground">
                        Track sessions for revocation
                      </span>
                      <span className="text-muted-foreground">
                        Persist auth and org access sessions in the database so they can be revoked
                        before their access token expires.
                      </span>
                    </span>
                  </label>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="data" className="flex flex-col gap-6">
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

                  <Field label="Query cursor page size" error={fieldErrors.query_cursor_page_size}>
                    <Input
                      aria-label="Query cursor page size"
                      aria-invalid={Boolean(fieldErrors.query_cursor_page_size) || undefined}
                      type="number"
                      min={1}
                      step={1}
                      value={form.query_cursor_page_size}
                      disabled={disabled}
                      onChange={(event) => {
                        const next = event.target.valueAsNumber
                        updateField('query_cursor_page_size', Number.isFinite(next) ? next : 0)
                      }}
                    />
                    <FieldDescription>
                      Default number of rows returned per cursor page before another fetch is
                      required.
                    </FieldDescription>
                  </Field>

                  <Field
                    label="Synchronous export limit"
                    error={fieldErrors.exports_sync_max_bytes}
                  >
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
                <CardDescription>
                  How often persisted schema snapshots are refreshed.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Field
                  label="Snapshot freshness"
                  error={fieldErrors.schema_snapshot_freshness_seconds}
                >
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
                  <label className="flex cursor-pointer items-start gap-3 py-2 has-disabled:cursor-not-allowed has-disabled:opacity-50">
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
                      disabled={
                        disabled || fileRevisionsUnsupported || !form.file_revisions_enabled
                      }
                      onChange={(event) => {
                        const next = event.target.valueAsNumber
                        updateField('file_revisions_keep_latest', Number.isFinite(next) ? next : 0)
                      }}
                    />
                  </Field>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="operations" className="flex flex-col gap-6">
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
                    onChange={(event) =>
                      updateField('error_notification_email', event.target.value)
                    }
                  />
                </Field>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Logging</CardTitle>
                <CardDescription>
                  Control log verbosity and query tracing. Changes apply immediately without an API
                  restart.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="Log level" error={fieldErrors.log_level}>
                    <Select
                      value={form.log_level}
                      onValueChange={(value) => updateField('log_level', value as LogLevel)}
                      disabled={disabled}
                    >
                      <SelectTrigger
                        aria-label="Log level"
                        aria-invalid={Boolean(fieldErrors.log_level) || undefined}
                        className="w-full"
                      >
                        <SelectValue>
                          {logLevelOptions.find((option) => option.value === form.log_level)
                            ?.label ?? form.log_level}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {logLevelOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>

                  <FieldRoot
                    orientation="horizontal"
                    data-invalid={Boolean(fieldErrors.database_query_tracing_enabled)}
                  >
                    <Checkbox
                      aria-label="Enable database query tracing"
                      checked={form.database_query_tracing_enabled}
                      disabled={disabled}
                      onCheckedChange={(checked) =>
                        updateField('database_query_tracing_enabled', checked === true)
                      }
                    />
                    <FieldContent>
                      <FieldLabel>Enable database query tracing</FieldLabel>
                      <FieldDescription>
                        Logs queries executed against SQLWarden's application database at debug
                        level. Set the log level to Debug and enable only temporarily.
                      </FieldDescription>
                      <FieldError>{fieldErrors.database_query_tracing_enabled}</FieldError>
                    </FieldContent>
                  </FieldRoot>

                  <FieldRoot
                    orientation="horizontal"
                    data-invalid={Boolean(fieldErrors.access_logs_enabled)}
                  >
                    <Checkbox
                      aria-label="Enable HTTP access logs"
                      checked={form.access_logs_enabled}
                      disabled={disabled}
                      onCheckedChange={(checked) =>
                        updateField('access_logs_enabled', checked === true)
                      }
                    />
                    <FieldContent>
                      <FieldLabel>Enable HTTP access logs</FieldLabel>
                      <FieldDescription>
                        Logs one entry for every completed HTTP request. Request IDs and error logs
                        remain available when this is disabled.
                      </FieldDescription>
                      <FieldError>{fieldErrors.access_logs_enabled}</FieldError>
                    </FieldContent>
                  </FieldRoot>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Background Jobs</CardTitle>
                <CardDescription>
                  Tune the background job runner. Changing these values drains in-flight jobs and
                  restarts the runner without restarting the API.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="Worker count" error={fieldErrors.jobs_worker_count}>
                    <Input
                      aria-label="Worker count"
                      aria-invalid={Boolean(fieldErrors.jobs_worker_count) || undefined}
                      type="number"
                      min={1}
                      max={256}
                      step={1}
                      value={form.jobs_worker_count}
                      disabled={disabled}
                      onChange={(event) => {
                        const next = event.target.valueAsNumber
                        updateField('jobs_worker_count', Number.isFinite(next) ? next : 0)
                      }}
                    />
                  </Field>

                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field label="Poll interval" error={fieldErrors.jobs_poll_interval_seconds}>
                      <UnitInputField
                        label="Poll interval"
                        error={Boolean(fieldErrors.jobs_poll_interval_seconds)}
                        amount={secondsInUnit(
                          form.jobs_poll_interval_seconds,
                          units.jobsPollInterval,
                        )}
                        unit={units.jobsPollInterval}
                        options={durationUnitOptions}
                        disabled={disabled}
                        min={secondsInUnit(1, units.jobsPollInterval)}
                        max={secondsInUnit(3_600, units.jobsPollInterval)}
                        onAmountChange={(amount) =>
                          updateField(
                            'jobs_poll_interval_seconds',
                            durationToSeconds(amount, units.jobsPollInterval),
                          )
                        }
                        onUnitChange={(unit) =>
                          setUnits((current) => ({ ...current, jobsPollInterval: unit }))
                        }
                      />
                    </Field>
                    <Field label="Claim lease" error={fieldErrors.jobs_claim_lease_seconds}>
                      <UnitInputField
                        label="Claim lease"
                        error={Boolean(fieldErrors.jobs_claim_lease_seconds)}
                        amount={secondsInUnit(form.jobs_claim_lease_seconds, units.jobsClaimLease)}
                        unit={units.jobsClaimLease}
                        options={durationUnitOptions}
                        disabled={disabled}
                        min={secondsInUnit(1, units.jobsClaimLease)}
                        max={secondsInUnit(86_400, units.jobsClaimLease)}
                        onAmountChange={(amount) =>
                          updateField(
                            'jobs_claim_lease_seconds',
                            durationToSeconds(amount, units.jobsClaimLease),
                          )
                        }
                        onUnitChange={(unit) =>
                          setUnits((current) => ({ ...current, jobsClaimLease: unit }))
                        }
                      />
                    </Field>
                  </div>

                  <Field
                    label="Completed job retention"
                    error={fieldErrors.jobs_completed_retention_seconds}
                  >
                    <UnitInputField
                      label="Completed job retention"
                      error={Boolean(fieldErrors.jobs_completed_retention_seconds)}
                      amount={secondsInUnit(
                        form.jobs_completed_retention_seconds,
                        units.jobsCompletedRetention,
                      )}
                      unit={units.jobsCompletedRetention}
                      options={durationUnitOptions}
                      disabled={disabled}
                      min={secondsInUnit(1, units.jobsCompletedRetention)}
                      max={secondsInUnit(31_536_000, units.jobsCompletedRetention)}
                      onAmountChange={(amount) =>
                        updateField(
                          'jobs_completed_retention_seconds',
                          durationToSeconds(amount, units.jobsCompletedRetention),
                        )
                      }
                      onUnitChange={(unit) =>
                        setUnits((current) => ({ ...current, jobsCompletedRetention: unit }))
                      }
                    />
                    <p className="text-xs text-muted-foreground">
                      Completed jobs older than this are purged.
                    </p>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="email" className="flex flex-col gap-6">
            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Email Delivery</CardTitle>
                <CardDescription>
                  SMTP settings used to send outbound notification email.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <FieldRoot
                    orientation="horizontal"
                    data-invalid={Boolean(fieldErrors.smtp_enabled)}
                  >
                    <Checkbox
                      aria-label="Enable SMTP"
                      checked={form.smtp_enabled}
                      disabled={disabled}
                      onCheckedChange={(checked) => updateField('smtp_enabled', checked === true)}
                    />
                    <FieldContent>
                      <FieldLabel>Enable SMTP</FieldLabel>
                      <FieldDescription>
                        Send outbound email through the configured SMTP server.
                      </FieldDescription>
                      <FieldError>{fieldErrors.smtp_enabled}</FieldError>
                    </FieldContent>
                  </FieldRoot>

                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field label="SMTP host" error={fieldErrors.smtp_host}>
                      <Input
                        aria-label="SMTP host"
                        aria-invalid={Boolean(fieldErrors.smtp_host) || undefined}
                        value={form.smtp_host}
                        placeholder="smtp.example.com"
                        disabled={disabled || !form.smtp_enabled}
                        onChange={(event) => updateField('smtp_host', event.target.value)}
                      />
                    </Field>
                    <Field label="SMTP port" error={fieldErrors.smtp_port}>
                      <Input
                        aria-label="SMTP port"
                        aria-invalid={Boolean(fieldErrors.smtp_port) || undefined}
                        type="number"
                        min={1}
                        max={65_535}
                        step={1}
                        value={form.smtp_port}
                        disabled={disabled || !form.smtp_enabled}
                        onChange={(event) => {
                          const next = event.target.valueAsNumber
                          updateField('smtp_port', Number.isFinite(next) ? next : 0)
                        }}
                      />
                    </Field>
                  </div>

                  <Field label="SMTP username" error={fieldErrors.smtp_username}>
                    <Input
                      aria-label="SMTP username"
                      aria-invalid={Boolean(fieldErrors.smtp_username) || undefined}
                      value={form.smtp_username}
                      disabled={disabled || !form.smtp_enabled}
                      onChange={(event) => updateField('smtp_username', event.target.value)}
                    />
                  </Field>

                  <Field label="SMTP password" error={fieldErrors.smtp_password}>
                    <PasswordInput
                      aria-label="SMTP password"
                      aria-invalid={Boolean(fieldErrors.smtp_password) || undefined}
                      value={smtpPasswordInputValue(smtpPasswordAction)}
                      placeholder={
                        smtpPasswordAction.kind === 'clear'
                          ? 'Password will be cleared'
                          : form.smtp_password_configured
                            ? 'Leave blank to keep the saved password'
                            : 'No password configured'
                      }
                      disabled={
                        disabled || !form.smtp_enabled || smtpPasswordAction.kind === 'clear'
                      }
                      onChange={(event) =>
                        updateSmtpPasswordAction(
                          event.target.value === ''
                            ? { kind: 'unchanged' }
                            : { kind: 'set', value: event.target.value },
                        )
                      }
                    />
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-xs text-muted-foreground">
                        {smtpPasswordStatusText(smtpPasswordAction, form.smtp_password_configured)}
                      </p>
                      {form.smtp_password_configured ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          disabled={disabled || !form.smtp_enabled}
                          onClick={() =>
                            updateSmtpPasswordAction(
                              smtpPasswordAction.kind === 'clear'
                                ? { kind: 'unchanged' }
                                : { kind: 'clear' },
                            )
                          }
                        >
                          {smtpPasswordAction.kind === 'clear'
                            ? 'Keep saved password'
                            : 'Clear saved password'}
                        </Button>
                      ) : null}
                    </div>
                  </Field>

                  <Field label="Sender address" error={fieldErrors.smtp_from}>
                    <Input
                      aria-label="Sender address"
                      aria-invalid={Boolean(fieldErrors.smtp_from) || undefined}
                      value={form.smtp_from}
                      placeholder="SQLWarden <noreply@example.com>"
                      disabled={disabled || !form.smtp_enabled}
                      onChange={(event) => updateField('smtp_from', event.target.value)}
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="deployment" className="flex flex-col gap-6">
            <DeploymentConfiguration
              configuration={configuration.data}
              isLoading={configuration.isLoading}
              isError={configuration.isError}
            />
          </TabsContent>
        </Tabs>
      </form>
    </div>
  )
}

function DeploymentConfiguration({
  configuration,
  isLoading,
  isError,
}: {
  configuration?: InstanceConfiguration
  isLoading: boolean
  isError: boolean
}) {
  if (isLoading) {
    return (
      <Alert>
        <AlertTitle>Loading deployment configuration</AlertTitle>
        <AlertDescription>
          Reading the values supplied when this SQLWarden process started.
        </AlertDescription>
      </Alert>
    )
  }

  if (isError || !configuration) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Deployment configuration unavailable</AlertTitle>
        <AlertDescription>
          Runtime settings remain editable, but deployment-managed values could not be loaded.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <>
      <Alert>
        <AlertTitle className="flex flex-wrap items-center gap-2">
          Deployment-managed configuration
          <Badge variant={configuration.restart_required ? 'destructive' : 'secondary'}>
            {configuration.restart_required ? 'Restart required' : 'Managed externally'}
          </Badge>
        </AlertTitle>
        <AlertDescription>
          These values are read-only in SQLWarden. They are resolved at process startup from the
          configuration file, environment variables, and CLI flags. Update the deployment source and
          restart SQLWarden to change them.
        </AlertDescription>
      </Alert>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card size="sm">
          <CardHeader>
            <CardTitle>Networking</CardTitle>
            <CardDescription>Listener and process mode.</CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col">
              <ConfigurationRow label="HTTP port" value={String(configuration.http_port)} />
              <ConfigurationRow
                label="TLS enabled"
                value={configuration.tls_enabled ? 'Yes' : 'No'}
              />
              <ConfigurationRow label="Mode" value={configuration.mode} />
            </dl>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Access &amp; logging</CardTitle>
            <CardDescription>Bootstrap behavior for this process.</CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col">
              <ConfigurationRow label="Log format" value={configuration.log_format} />
            </dl>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Storage</CardTitle>
            <CardDescription>Application metadata and file backends.</CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col">
              <ConfigurationRow label="Database driver" value={configuration.database_driver} />
              <ConfigurationRow
                label="Database auto-migrate"
                value={configuration.database_automigrate ? 'Yes' : 'No'}
              />
              <ConfigurationRow label="File storage mode" value={configuration.file_storage_mode} />
              <ConfigurationRow
                label="File storage backend"
                value={configuration.file_storage_backend}
              />
              <ConfigurationRow
                label="Local SQLite sources"
                value={configuration.sqlite_local_enabled ? 'Allowed' : 'Disabled'}
              />
            </dl>
          </CardContent>
        </Card>
      </div>
    </>
  )
}

function ConfigurationRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 first:pt-0 last:pb-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate text-right font-mono text-foreground" title={value}>
        {value}
      </dd>
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
