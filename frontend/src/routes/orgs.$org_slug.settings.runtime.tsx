import { errorMessage, isApiError } from '#/lib/api/errors'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Navigate, createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import {
  orgEffectivePermissionsQueryOptions,
  orgRuntimeSettingsQueryOptions,
  queryKeys,
} from '#/lib/api/query'
import {
  buildRuntimeSettingsPatch,
  hasRuntimeSettingsChanges,
  runtimeSettingsFormState,
  type RuntimeSettingsFormState,
} from '#/lib/api/runtime-settings-form'
import type {
  OrganizationRuntimeOverrideValues,
  OrganizationRuntimeSettings,
} from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import {
  bytesInUnit,
  bytesToSize,
  byteUnitOptions,
  durationToSeconds,
  durationUnitOptions,
  formatBytesValue,
  formatDuration,
  secondsInUnit,
  secondsToDuration,
  sizeToBytes,
  type ByteUnit,
  type DurationUnit,
} from '#/lib/units'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Checkbox } from '#/components/ui/checkbox'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { RoutePending } from '#/components/RoutePending'
import { OverrideField } from '#/components/settings/OverrideField'
import { UnitInputField } from '#/components/settings/UnitInputField'

export const Route = createFileRoute('/orgs/$org_slug/settings/runtime')({
  component: LegacyRuntimeSettingsRedirect,
})

function LegacyRuntimeSettingsRedirect() {
  const { org_slug: orgSlug } = Route.useParams()
  return (
    <Navigate
      to="/orgs/$org_slug/settings/general"
      params={{ org_slug: orgSlug }}
      search={{ tab: 'runtime' }}
      replace
    />
  )
}

export interface OrganizationSettingsActionState {
  disabled: boolean
  hasChanges: boolean
  pending: boolean
}

type RuntimeFieldErrors = Partial<Record<keyof OrganizationRuntimeOverrideValues, string>>

interface RuntimeUnits {
  queryMaxResultBytes: ByteUnit
  exportsSyncMaxBytes: ByteUnit
  exportsBackgroundMaxBytes: ByteUnit
  schemaSnapshotFreshnessSeconds: DurationUnit
}

function unitsFromForm(form: RuntimeSettingsFormState): RuntimeUnits {
  return {
    queryMaxResultBytes: bytesToSize(form.queryMaxResultBytes.value).unit,
    exportsSyncMaxBytes: bytesToSize(form.exportsSyncMaxBytes.value).unit,
    exportsBackgroundMaxBytes: bytesToSize(form.exportsBackgroundMaxBytes.value).unit,
    schemaSnapshotFreshnessSeconds: secondsToDuration(form.schemaSnapshotFreshnessSeconds.value)
      .unit,
  }
}

export function OrganizationRuntimeSettingsPanel({
  orgSlug,
  formId,
  onActionStateChange,
}: {
  orgSlug: string
  formId: string
  onActionStateChange: (state: OrganizationSettingsActionState) => void
}) {
  const queryClient = useQueryClient()
  const settings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const [form, setForm] = useState<RuntimeSettingsFormState | null>(null)
  const [units, setUnits] = useState<RuntimeUnits | null>(null)
  const [fieldErrors, setFieldErrors] = useState<RuntimeFieldErrors>({})

  const canWrite = hasPermission(effectivePermissions.data?.permissions, permission.orgWrite)

  useEffect(() => {
    if (!settings.data) return
    const nextForm = runtimeSettingsFormState(settings.data)
    setForm(nextForm)
    setUnits(unitsFromForm(nextForm))
  }, [settings.data])

  useEffect(() => {
    if (!settings.error) return
    if (isApiError(settings.error) && settings.error.code === 'settings_unavailable') return
    toast.error(errorMessage(settings.error, 'Failed to load data controls'))
  }, [settings.error])

  const updateSettings = useMutation({
    mutationFn: async () => {
      if (!settings.data || !form) throw new Error('Data controls are not loaded yet.')
      const patch = buildRuntimeSettingsPatch(form, settings.data.overrides)
      return api.patch<OrganizationRuntimeSettings>(
        `/api/v1/orgs/${orgSlug}/runtime-settings`,
        patch,
      )
    },
    onSuccess: async (updated) => {
      setFieldErrors({})
      const nextForm = runtimeSettingsFormState(updated)
      setForm(nextForm)
      setUnits(unitsFromForm(nextForm))
      toast.success('Data controls updated')
      queryClient.setQueryData(queryKeys.orgRuntimeSettings(orgSlug), updated)
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors(error.fieldErrors as RuntimeFieldErrors)
        return
      }
      toast.error(errorMessage(error, 'Failed to update data controls'))
    },
  })

  const hasChanges = Boolean(
    form && settings.data && hasRuntimeSettingsChanges(form, settings.data.overrides),
  )
  const disabled = !canWrite || updateSettings.isPending

  useEffect(() => {
    onActionStateChange({
      disabled: disabled || !form || !settings.data,
      hasChanges,
      pending: updateSettings.isPending,
    })
  }, [disabled, form, hasChanges, onActionStateChange, settings.data, updateSettings.isPending])

  if (settings.isLoading) {
    return <RoutePending />
  }

  if (isApiError(settings.error) && settings.error.code === 'settings_unavailable') {
    return (
      <div className="flex flex-col gap-4">
        <Alert variant="destructive">
          <AlertTitle>Settings unavailable</AlertTitle>
          <AlertDescription>
            Runtime settings are temporarily unavailable. Try again shortly.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  if (settings.isError || !settings.data || !form || !units) {
    return <p className="text-muted-foreground">Failed to load data controls.</p>
  }

  const { constraints } = settings.data
  const fileRevisionsDisabled = disabled || !constraints.file_revisions_available

  function updateForm<K extends keyof RuntimeSettingsFormState>(
    field: K,
    value: RuntimeSettingsFormState[K],
  ) {
    setForm((current) => (current ? { ...current, [field]: value } : current))
    setFieldErrors((current) => ({
      ...current,
      [runtimeFormFieldToApiField[field]]: undefined,
    }))
  }

  function submitSettings(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void updateSettings.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-6">
      <form id={formId} className="flex flex-col gap-6" onSubmit={submitSettings}>
        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Query &amp; Export Limits</CardTitle>
            <CardDescription>
              Bound how much data a query or export can return for this organization.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <OverrideField
              label="Query row limit"
              description="Maximum rows returned by a single query."
              overridden={form.queryMaxResultRows.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('queryMaxResultRows', {
                  ...form.queryMaxResultRows,
                  overridden: false,
                  value: constraints.query_max_result_rows_max,
                })
              }
              limitText={`${constraints.query_max_result_rows_max.toLocaleString()} rows`}
              limitDescription="This fixed number is the instance-wide maximum. The organization can use the same or a lower row limit."
              error={fieldErrors.query_max_result_rows}
            >
              <Input
                aria-label="Query row limit"
                aria-invalid={Boolean(fieldErrors.query_max_result_rows) || undefined}
                type="number"
                min={1}
                max={constraints.query_max_result_rows_max}
                step={1}
                value={form.queryMaxResultRows.value}
                disabled={disabled}
                onChange={(event) => {
                  const next = event.target.valueAsNumber
                  updateForm('queryMaxResultRows', {
                    ...form.queryMaxResultRows,
                    overridden: true,
                    value: Number.isFinite(next) ? next : 0,
                  })
                }}
              />
            </OverrideField>

            <OverrideField
              label="Query byte limit"
              description="Maximum result size returned by a single query."
              overridden={form.queryMaxResultBytes.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('queryMaxResultBytes', {
                  ...form.queryMaxResultBytes,
                  overridden: false,
                  value: constraints.query_max_result_bytes_max,
                })
              }
              limitText={formatBytesValue(constraints.query_max_result_bytes_max)}
              limitDescription="This fixed value is the instance-wide maximum result size. The organization can only choose a smaller limit."
              error={fieldErrors.query_max_result_bytes}
            >
              <UnitInputField
                label="Query byte limit"
                error={Boolean(fieldErrors.query_max_result_bytes)}
                amount={bytesInUnit(form.queryMaxResultBytes.value, units.queryMaxResultBytes)}
                unit={units.queryMaxResultBytes}
                options={byteUnitOptions}
                disabled={disabled}
                min={bytesInUnit(1, units.queryMaxResultBytes)}
                max={bytesInUnit(constraints.query_max_result_bytes_max, units.queryMaxResultBytes)}
                onAmountChange={(amount) =>
                  updateForm('queryMaxResultBytes', {
                    ...form.queryMaxResultBytes,
                    overridden: true,
                    value: sizeToBytes(amount, units.queryMaxResultBytes),
                  })
                }
                onUnitChange={(unit) =>
                  setUnits((current) =>
                    current ? { ...current, queryMaxResultBytes: unit } : current,
                  )
                }
              />
            </OverrideField>

            <OverrideField
              label="Synchronous export limit"
              description="Maximum size for an export completed inline within the request."
              overridden={form.exportsSyncMaxBytes.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('exportsSyncMaxBytes', {
                  ...form.exportsSyncMaxBytes,
                  overridden: false,
                  value: constraints.exports_sync_max_bytes_max,
                })
              }
              limitText={formatBytesValue(constraints.exports_sync_max_bytes_max)}
              limitDescription="This fixed value is the instance-wide maximum synchronous export size. The organization can only choose a smaller limit."
              error={fieldErrors.exports_sync_max_bytes}
            >
              <UnitInputField
                label="Synchronous export limit"
                error={Boolean(fieldErrors.exports_sync_max_bytes)}
                amount={bytesInUnit(form.exportsSyncMaxBytes.value, units.exportsSyncMaxBytes)}
                unit={units.exportsSyncMaxBytes}
                options={byteUnitOptions}
                disabled={disabled}
                min={bytesInUnit(1, units.exportsSyncMaxBytes)}
                max={bytesInUnit(constraints.exports_sync_max_bytes_max, units.exportsSyncMaxBytes)}
                onAmountChange={(amount) =>
                  updateForm('exportsSyncMaxBytes', {
                    ...form.exportsSyncMaxBytes,
                    overridden: true,
                    value: sizeToBytes(amount, units.exportsSyncMaxBytes),
                  })
                }
                onUnitChange={(unit) =>
                  setUnits((current) =>
                    current ? { ...current, exportsSyncMaxBytes: unit } : current,
                  )
                }
              />
            </OverrideField>

            <OverrideField
              label="Background export limit"
              description={
                constraints.exports_background_max_bytes_max === 0
                  ? 'Maximum size for an export processed as a background job. 0 means unlimited.'
                  : 'Maximum size for an export processed as a background job.'
              }
              overridden={form.exportsBackgroundMaxBytes.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('exportsBackgroundMaxBytes', {
                  ...form.exportsBackgroundMaxBytes,
                  overridden: false,
                  value: constraints.exports_background_max_bytes_max,
                })
              }
              limitText={
                constraints.exports_background_max_bytes_max === 0
                  ? 'Unlimited'
                  : formatBytesValue(constraints.exports_background_max_bytes_max)
              }
              limitDescription="This fixed value is the instance-wide background export limit. A finite instance limit cannot be relaxed by the organization."
              error={fieldErrors.exports_background_max_bytes}
            >
              <UnitInputField
                label="Background export limit"
                error={Boolean(fieldErrors.exports_background_max_bytes)}
                amount={bytesInUnit(
                  form.exportsBackgroundMaxBytes.value,
                  units.exportsBackgroundMaxBytes,
                )}
                unit={units.exportsBackgroundMaxBytes}
                options={byteUnitOptions}
                disabled={disabled}
                min={
                  constraints.exports_background_max_bytes_max === 0
                    ? 0
                    : bytesInUnit(1, units.exportsBackgroundMaxBytes)
                }
                max={
                  constraints.exports_background_max_bytes_max === 0
                    ? undefined
                    : bytesInUnit(
                        constraints.exports_background_max_bytes_max,
                        units.exportsBackgroundMaxBytes,
                      )
                }
                onAmountChange={(amount) =>
                  updateForm('exportsBackgroundMaxBytes', {
                    ...form.exportsBackgroundMaxBytes,
                    overridden: true,
                    value: sizeToBytes(amount, units.exportsBackgroundMaxBytes),
                  })
                }
                onUnitChange={(unit) =>
                  setUnits((current) =>
                    current ? { ...current, exportsBackgroundMaxBytes: unit } : current,
                  )
                }
              />
            </OverrideField>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Schema Snapshots</CardTitle>
            <CardDescription>
              How often persisted schema snapshots refresh for this organization.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <OverrideField
              label="Snapshot freshness"
              description="Minimum interval between snapshot refreshes. Can only be made less frequent than the instance interval."
              overridden={form.schemaSnapshotFreshnessSeconds.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('schemaSnapshotFreshnessSeconds', {
                  ...form.schemaSnapshotFreshnessSeconds,
                  overridden: false,
                  value: constraints.schema_snapshot_freshness_seconds_min,
                })
              }
              limitText={formatDuration(constraints.schema_snapshot_freshness_seconds_min)}
              limitDescription="This fixed value is the instance-wide minimum refresh interval. The organization can refresh less frequently, but not more frequently."
              error={fieldErrors.schema_snapshot_freshness_seconds}
            >
              <UnitInputField
                label="Snapshot freshness"
                error={Boolean(fieldErrors.schema_snapshot_freshness_seconds)}
                amount={secondsInUnit(
                  form.schemaSnapshotFreshnessSeconds.value,
                  units.schemaSnapshotFreshnessSeconds,
                )}
                unit={units.schemaSnapshotFreshnessSeconds}
                options={durationUnitOptions}
                disabled={disabled}
                min={secondsInUnit(
                  constraints.schema_snapshot_freshness_seconds_min,
                  units.schemaSnapshotFreshnessSeconds,
                )}
                onAmountChange={(amount) =>
                  updateForm('schemaSnapshotFreshnessSeconds', {
                    ...form.schemaSnapshotFreshnessSeconds,
                    overridden: true,
                    value: durationToSeconds(amount, units.schemaSnapshotFreshnessSeconds),
                  })
                }
                onUnitChange={(unit) =>
                  setUnits((current) =>
                    current ? { ...current, schemaSnapshotFreshnessSeconds: unit } : current,
                  )
                }
              />
            </OverrideField>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>File Revisions</CardTitle>
            <CardDescription>
              Keep prior revisions of saved workspace files for this organization.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <OverrideField
              label="Enable file revisions"
              description="Organizations can only further restrict this; it cannot be enabled here if disabled at the instance level."
              overridden={form.fileRevisionsEnabled.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('fileRevisionsEnabled', {
                  ...form.fileRevisionsEnabled,
                  overridden: false,
                  value: constraints.file_revisions_available,
                })
              }
              limitText={constraints.file_revisions_available ? 'Enabled' : 'Disabled'}
              limitDescription="This fixed state comes from the instance setting. The organization may disable revisions, but cannot enable them when the instance has disabled them."
              error={fieldErrors.file_revisions_enabled}
            >
              <div className="flex items-center gap-2 text-sm">
                <Checkbox
                  aria-label="File revisions enabled"
                  checked={form.fileRevisionsEnabled.value}
                  disabled={fileRevisionsDisabled}
                  onCheckedChange={(checked) =>
                    updateForm('fileRevisionsEnabled', {
                      ...form.fileRevisionsEnabled,
                      overridden: true,
                      value: checked === true,
                    })
                  }
                />
                {form.fileRevisionsEnabled.value ? 'Enabled' : 'Disabled'}
              </div>
            </OverrideField>

            <OverrideField
              label="Revisions to keep"
              description="Maximum stored revisions per file."
              overridden={form.fileRevisionsKeepLatest.overridden}
              disabled={disabled}
              onReset={() =>
                updateForm('fileRevisionsKeepLatest', {
                  ...form.fileRevisionsKeepLatest,
                  overridden: false,
                  value: constraints.file_revisions_keep_latest_max,
                })
              }
              limitText={`${constraints.file_revisions_keep_latest_max.toLocaleString()} revisions`}
              limitDescription="This fixed number is the instance-wide maximum. The organization can retain the same or fewer revisions per file."
              error={fieldErrors.file_revisions_keep_latest}
            >
              <Input
                aria-label="Revisions to keep"
                aria-invalid={Boolean(fieldErrors.file_revisions_keep_latest) || undefined}
                type="number"
                min={0}
                max={constraints.file_revisions_keep_latest_max}
                step={1}
                value={form.fileRevisionsKeepLatest.value}
                disabled={fileRevisionsDisabled}
                onChange={(event) => {
                  const next = event.target.valueAsNumber
                  updateForm('fileRevisionsKeepLatest', {
                    ...form.fileRevisionsKeepLatest,
                    overridden: true,
                    value: Number.isFinite(next) ? next : 0,
                  })
                }}
              />
            </OverrideField>
          </CardContent>
        </Card>

        {!canWrite ? (
          <p className="text-xs text-muted-foreground">
            You need organization write permission to change data controls.
          </p>
        ) : null}
      </form>
    </div>
  )
}

const runtimeFormFieldToApiField: Record<
  keyof RuntimeSettingsFormState,
  keyof OrganizationRuntimeOverrideValues
> = {
  queryMaxResultRows: 'query_max_result_rows',
  queryMaxResultBytes: 'query_max_result_bytes',
  exportsSyncMaxBytes: 'exports_sync_max_bytes',
  exportsBackgroundMaxBytes: 'exports_background_max_bytes',
  schemaSnapshotFreshnessSeconds: 'schema_snapshot_freshness_seconds',
  fileRevisionsEnabled: 'file_revisions_enabled',
  fileRevisionsKeepLatest: 'file_revisions_keep_latest',
}
