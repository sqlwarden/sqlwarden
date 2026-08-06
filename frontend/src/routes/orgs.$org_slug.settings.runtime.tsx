import { errorMessage, isApiError } from '#/lib/api/errors'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
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
import { Button } from '#/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { RoutePending } from '#/components/RoutePending'
import { OverrideField } from '#/components/settings/OverrideField'
import { UnitInputField } from '#/components/settings/UnitInputField'

export const Route = createFileRoute('/orgs/$org_slug/settings/runtime')({
  component: OrganizationRuntimeSettingsPage,
  pendingComponent: RoutePending,
})

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

function OrganizationRuntimeSettingsPage() {
  const { org_slug: orgSlug } = Route.useParams()
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
    toast.error(errorMessage(settings.error, 'Failed to load runtime policy'))
  }, [settings.error])

  const updateSettings = useMutation({
    mutationFn: async () => {
      if (!settings.data || !form) throw new Error('Runtime policy is not loaded yet.')
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
      toast.success('Runtime policy updated')
      queryClient.setQueryData(queryKeys.orgRuntimeSettings(orgSlug), updated)
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors(error.fieldErrors as RuntimeFieldErrors)
        return
      }
      toast.error(errorMessage(error, 'Failed to update runtime policy'))
    },
  })

  if (settings.isLoading) {
    return <RoutePending />
  }

  if (isApiError(settings.error) && settings.error.code === 'settings_unavailable') {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Runtime Policy</h1>
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
    return (
      <div className="flex flex-col gap-1.5">
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Runtime Policy</h1>
        <p className="text-muted-foreground">Failed to load runtime policy.</p>
      </div>
    )
  }

  const { effective, constraints } = settings.data
  const disabled = !canWrite || updateSettings.isPending
  const fileRevisionsDisabled = disabled || !constraints.file_revisions_available
  const hasChanges = hasRuntimeSettingsChanges(form, settings.data.overrides)

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
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Runtime Policy</h1>
        <p className="text-sm text-muted-foreground">
          Override instance-wide query, export, and storage limits for this organization. Overrides
          can only tighten instance limits, never relax them.
        </p>
      </div>

      <form className="flex flex-col gap-8" onSubmit={submitSettings}>
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
              onOverrideChange={(overridden) =>
                updateForm('queryMaxResultRows', { ...form.queryMaxResultRows, overridden })
              }
              effectiveText={`${effective.query_max_result_rows.toLocaleString()} rows`}
              constraintText={`Instance limit: ${constraints.query_max_result_rows_max.toLocaleString()} rows`}
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
              onOverrideChange={(overridden) =>
                updateForm('queryMaxResultBytes', { ...form.queryMaxResultBytes, overridden })
              }
              effectiveText={formatBytesValue(effective.query_max_result_bytes)}
              constraintText={`Instance limit: ${formatBytesValue(constraints.query_max_result_bytes_max)}`}
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
              onOverrideChange={(overridden) =>
                updateForm('exportsSyncMaxBytes', { ...form.exportsSyncMaxBytes, overridden })
              }
              effectiveText={formatBytesValue(effective.exports_sync_max_bytes)}
              constraintText={`Instance limit: ${formatBytesValue(constraints.exports_sync_max_bytes_max)}`}
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
              onOverrideChange={(overridden) =>
                updateForm('exportsBackgroundMaxBytes', {
                  ...form.exportsBackgroundMaxBytes,
                  overridden,
                })
              }
              effectiveText={
                effective.exports_background_max_bytes === 0
                  ? 'Unlimited'
                  : formatBytesValue(effective.exports_background_max_bytes)
              }
              constraintText={
                constraints.exports_background_max_bytes_max === 0
                  ? 'Instance limit: unlimited'
                  : `Instance limit: ${formatBytesValue(constraints.exports_background_max_bytes_max)}`
              }
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
              onOverrideChange={(overridden) =>
                updateForm('schemaSnapshotFreshnessSeconds', {
                  ...form.schemaSnapshotFreshnessSeconds,
                  overridden,
                })
              }
              effectiveText={formatDuration(effective.schema_snapshot_freshness_seconds)}
              constraintText={`Instance minimum interval: ${formatDuration(constraints.schema_snapshot_freshness_seconds_min)}`}
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
              onOverrideChange={(overridden) =>
                updateForm('fileRevisionsEnabled', { ...form.fileRevisionsEnabled, overridden })
              }
              effectiveText={effective.file_revisions_enabled ? 'Enabled' : 'Disabled'}
              constraintText={
                constraints.file_revisions_available
                  ? undefined
                  : 'Disabled at the instance level; cannot be enabled here.'
              }
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
              onOverrideChange={(overridden) =>
                updateForm('fileRevisionsKeepLatest', {
                  ...form.fileRevisionsKeepLatest,
                  overridden,
                })
              }
              effectiveText={`${effective.file_revisions_keep_latest.toLocaleString()} revisions`}
              constraintText={`Instance limit: ${constraints.file_revisions_keep_latest_max.toLocaleString()} revisions`}
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
                    value: Number.isFinite(next) ? next : 0,
                  })
                }}
              />
            </OverrideField>
          </CardContent>
        </Card>

        {!canWrite ? (
          <p className="text-xs text-muted-foreground">
            You need organization write permission to change runtime policy.
          </p>
        ) : null}

        <div className="flex justify-end">
          <Button type="submit" disabled={disabled || !hasChanges}>
            {updateSettings.isPending ? 'Saving...' : 'Save changes'}
          </Button>
        </div>
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
