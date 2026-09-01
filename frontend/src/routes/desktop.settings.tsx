import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Navigate, createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { RoutePending } from '#/components/RoutePending'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '#/components/ui/field'
import { Input } from '#/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { instanceSettingsQueryOptions, orgQueryOptions, queryKeys } from '#/lib/api/query'
import type { InstanceSettings, Organization } from '#/lib/api/types'
import { useDesktopRuntime } from '#/lib/desktop/context'
import {
  getDesktopInfo,
  revealDesktopDataDirectory,
  revealDesktopLogDirectory,
  type DesktopInfo,
} from '#/lib/desktop/runtime'
import { Icon } from '#/lib/icons'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/desktop/settings')({ component: DesktopSettingsPage })

interface DesktopSettingsForm {
  schemaSnapshotsEnabled: boolean
  maskConnectionCredentialsOnEdit: boolean
  queryMaxResultRows: string
  queryMaxResultBytes: string
  queryCursorPageSize: string
  schemaSnapshotFreshnessSeconds: string
}

type FormErrors = Partial<Record<keyof DesktopSettingsForm, string>>

function formFrom(org: Organization, settings: InstanceSettings): DesktopSettingsForm {
  return {
    schemaSnapshotsEnabled: org.schema_snapshots_enabled ?? true,
    maskConnectionCredentialsOnEdit: org.mask_connection_credentials_on_edit ?? false,
    queryMaxResultRows: String(settings.query_max_result_rows),
    queryMaxResultBytes: String(settings.query_max_result_bytes),
    queryCursorPageSize: String(settings.query_cursor_page_size),
    schemaSnapshotFreshnessSeconds: String(settings.schema_snapshot_freshness_seconds),
  }
}

function DesktopSettingsPage() {
  usePageTitle('Settings')
  const setup = useSetupStatus()
  const desktop = useDesktopRuntime()
  const orgSlug = desktop.session?.identity.org_slug ?? ''

  if (setup.isLoading) return <RoutePending />
  if (setup.data?.mode !== 'desktop' || !desktop.native || !orgSlug) {
    return <Navigate to="/" replace />
  }
  return <DesktopSettingsContent orgSlug={orgSlug} />
}

export function DesktopSettingsContent({ orgSlug }: { orgSlug: string }) {
  const queryClient = useQueryClient()
  const org = useQuery(orgQueryOptions(orgSlug))
  const settings = useQuery(instanceSettingsQueryOptions())
  const [form, setForm] = useState<DesktopSettingsForm | null>(null)
  const [original, setOriginal] = useState<DesktopSettingsForm | null>(null)
  const [fieldErrors, setFieldErrors] = useState<FormErrors>({})
  const [info, setInfo] = useState<DesktopInfo>()

  useEffect(() => {
    if (!org.data || !settings.data) return
    const next = formFrom(org.data, settings.data)
    setForm(next)
    setOriginal(next)
  }, [org.data, settings.data])

  useEffect(() => {
    void getDesktopInfo().then(setInfo)
  }, [])

  const changed = useMemo(
    () => Boolean(form && original && JSON.stringify(form) !== JSON.stringify(original)),
    [form, original],
  )

  const save = useMutation({
    mutationFn: async () => {
      if (!form || !original) return
      const validation = validate(form)
      if (Object.keys(validation).length > 0) {
        setFieldErrors(validation)
        throw new LocalValidationError()
      }

      const orgPatch: Partial<Organization> = {}
      if (form.schemaSnapshotsEnabled !== original.schemaSnapshotsEnabled)
        orgPatch.schema_snapshots_enabled = form.schemaSnapshotsEnabled
      if (form.maskConnectionCredentialsOnEdit !== original.maskConnectionCredentialsOnEdit)
        orgPatch.mask_connection_credentials_on_edit = form.maskConnectionCredentialsOnEdit

      const instancePatch: Partial<InstanceSettings> = {}
      const numericFields = [
        ['queryMaxResultRows', 'query_max_result_rows'],
        ['queryMaxResultBytes', 'query_max_result_bytes'],
        ['queryCursorPageSize', 'query_cursor_page_size'],
        ['schemaSnapshotFreshnessSeconds', 'schema_snapshot_freshness_seconds'],
      ] as const
      for (const [formKey, apiKey] of numericFields) {
        if (form[formKey] !== original[formKey]) instancePatch[apiKey] = Number(form[formKey])
      }

      await Promise.all([
        Object.keys(orgPatch).length
          ? api.patch<Organization>(`/api/v1/orgs/${orgSlug}`, orgPatch)
          : undefined,
        Object.keys(instancePatch).length
          ? api.patch<InstanceSettings>('/api/v1/instance/settings', instancePatch)
          : undefined,
      ])
    },
    onSuccess: async () => {
      setFieldErrors({})
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.org(orgSlug) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.instanceSettings() }),
      ])
      toast.success('Settings updated')
    },
    onError: (error) => {
      if (error instanceof LocalValidationError) return
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors(apiFieldErrors(error.fieldErrors))
        return
      }
      toast.error(errorMessage(error, 'Failed to update settings'))
    },
  })

  const unavailable =
    (isApiError(settings.error) && settings.error.code === 'settings_unavailable') ||
    (isApiError(org.error) && org.error.code === 'settings_unavailable')

  if (org.isLoading || settings.isLoading || !form || !original) return <RoutePending />
  if (unavailable) {
    return (
      <DesktopSettingsFrame orgSlug={orgSlug} saveDisabled>
        <Alert variant="destructive">
          <AlertTitle>Settings unavailable</AlertTitle>
          <AlertDescription>Settings are temporarily unavailable.</AlertDescription>
        </Alert>
      </DesktopSettingsFrame>
    )
  }
  if (org.isError || settings.isError) {
    return (
      <DesktopSettingsFrame orgSlug={orgSlug} saveDisabled>
        <p className="text-sm text-muted-foreground">Failed to load settings.</p>
      </DesktopSettingsFrame>
    )
  }

  function update<K extends keyof DesktopSettingsForm>(key: K, value: DesktopSettingsForm[K]) {
    setForm((current) => (current ? { ...current, [key]: value } : current))
    setFieldErrors((current) => ({ ...current, [key]: undefined }))
  }

  async function revealDirectory(action: () => Promise<void>, label: string) {
    try {
      await action()
    } catch (error) {
      toast.error(errorMessage(error, `Failed to reveal ${label}`))
    }
  }

  return (
    <DesktopSettingsFrame
      orgSlug={orgSlug}
      saveDisabled={!changed || save.isPending}
      savePending={save.isPending}
      onSave={() => save.mutate()}
    >
      <Tabs defaultValue="data" className="gap-5">
        <TabsList aria-label="Settings sections">
          <TabsTrigger value="data">Data</TabsTrigger>
          <TabsTrigger value="about">About &amp; Storage</TabsTrigger>
        </TabsList>
        <TabsContent value="data" className="space-y-4">
          <Card>
            <CardHeader className="border-b border-border">
              <CardTitle>Schema metadata</CardTitle>
              <CardDescription>
                Control metadata retained for local editor features.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <CheckboxField
                  id="schema-snapshots"
                  checked={form.schemaSnapshotsEnabled}
                  title="Persist schema snapshots"
                  description="Keep schema metadata between application sessions for faster browsing and completion."
                  onChange={(checked) => update('schemaSnapshotsEnabled', checked)}
                />
                <CheckboxField
                  id="mask-credentials"
                  checked={form.maskConnectionCredentialsOnEdit}
                  title="Mask connection credentials on edit"
                  description="Do not return saved secrets when a connection is opened for editing."
                  onChange={(checked) => update('maskConnectionCredentialsOnEdit', checked)}
                />
              </FieldGroup>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="border-b border-border">
              <CardTitle>Query limits</CardTitle>
              <CardDescription>
                Bound local query results and schema refresh behavior.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup className="grid gap-4 sm:grid-cols-2">
                <NumberField
                  label="Maximum result rows"
                  value={form.queryMaxResultRows}
                  error={fieldErrors.queryMaxResultRows}
                  onChange={(value) => update('queryMaxResultRows', value)}
                />
                <NumberField
                  label="Maximum result bytes"
                  value={form.queryMaxResultBytes}
                  error={fieldErrors.queryMaxResultBytes}
                  onChange={(value) => update('queryMaxResultBytes', value)}
                />
                <NumberField
                  label="Cursor page size"
                  value={form.queryCursorPageSize}
                  error={fieldErrors.queryCursorPageSize}
                  onChange={(value) => update('queryCursorPageSize', value)}
                />
                <NumberField
                  label="Schema freshness (seconds)"
                  value={form.schemaSnapshotFreshnessSeconds}
                  error={fieldErrors.schemaSnapshotFreshnessSeconds}
                  onChange={(value) => update('schemaSnapshotFreshnessSeconds', value)}
                />
              </FieldGroup>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="about">
          <Card>
            <CardHeader className="border-b border-border">
              <CardTitle>Application &amp; storage</CardTitle>
              <CardDescription>Native version and local data locations.</CardDescription>
            </CardHeader>
            <CardContent className="divide-y divide-border">
              <PathRow label="Version" value={info?.version ?? 'Unknown'} />
              <PathRow
                label="Data directory"
                value={info?.paths.data_dir}
                action="Reveal"
                onAction={() => void revealDirectory(revealDesktopDataDirectory, 'data directory')}
              />
              <PathRow label="Database" value={info?.paths.database} />
              <PathRow label="Files" value={info?.paths.files} />
              <PathRow
                label="Logs"
                value={info?.paths.logs}
                action="Reveal"
                onAction={() => void revealDirectory(revealDesktopLogDirectory, 'log directory')}
              />
              <PathRow label="Configuration" value={info?.paths.config_file} />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </DesktopSettingsFrame>
  )
}

function DesktopSettingsFrame({
  orgSlug,
  children,
  saveDisabled = false,
  savePending = false,
  onSave,
}: {
  orgSlug: string
  children: React.ReactNode
  saveDisabled?: boolean
  savePending?: boolean
  onSave?: () => void
}) {
  return (
    <main className="min-h-svh bg-background px-4 py-6 md:px-6">
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
        <header className="flex flex-wrap items-start justify-between gap-3 border-b border-border pb-4">
          <div className="space-y-1">
            <Button
              variant="ghost"
              size="sm"
              className="-ml-2"
              nativeButton={false}
              render={<Link to="/ide/$org_slug" params={{ org_slug: orgSlug }} />}
            >
              <Icon name="arrow-left-01" size={20} /> Back to editor
            </Button>
            <h1 className="font-heading text-xl font-semibold tracking-tight">Settings</h1>
            <p className="text-sm text-muted-foreground">
              Configure this local SQLWarden installation.
            </p>
          </div>
          {onSave ? (
            <Button onClick={onSave} disabled={saveDisabled}>
              {savePending ? 'Saving…' : 'Save changes'}
            </Button>
          ) : null}
        </header>
        {children}
      </div>
    </main>
  )
}

function CheckboxField({
  id,
  checked,
  title,
  description,
  onChange,
}: {
  id: string
  checked: boolean
  title: string
  description: string
  onChange: (checked: boolean) => void
}) {
  return (
    <Field orientation="horizontal">
      <Checkbox id={id} checked={checked} onCheckedChange={onChange} />
      <FieldContent>
        <FieldLabel htmlFor={id}>{title}</FieldLabel>
        <FieldDescription>{description}</FieldDescription>
      </FieldContent>
    </Field>
  )
}

function NumberField({
  label,
  value,
  error,
  onChange,
}: {
  label: string
  value: string
  error?: string
  onChange: (value: string) => void
}) {
  return (
    <Field data-invalid={Boolean(error)}>
      <FieldLabel>{label}</FieldLabel>
      <Input
        aria-label={label}
        type="number"
        min={1}
        step={1}
        value={value}
        aria-invalid={Boolean(error)}
        onChange={(event) => onChange(event.target.value)}
      />
      <FieldError>{error}</FieldError>
    </Field>
  )
}

function PathRow({
  label,
  value,
  action,
  onAction,
}: {
  label: string
  value?: string
  action?: string
  onAction?: () => void
}) {
  return (
    <div className="grid gap-2 py-3 first:pt-0 last:pb-0 sm:grid-cols-[9rem_minmax(0,1fr)_auto] sm:items-center">
      <FieldTitle>{label}</FieldTitle>
      <code
        className="min-w-0 truncate rounded bg-muted px-2 py-1 text-xs text-muted-foreground"
        title={value}
      >
        {value ?? 'Unavailable'}
      </code>
      {action && onAction ? (
        <Button variant="outline" size="sm" onClick={onAction}>
          <Icon name="folder-open" size={20} />
          {action}
        </Button>
      ) : null}
    </div>
  )
}

function validate(form: DesktopSettingsForm): FormErrors {
  const errors: FormErrors = {}
  for (const key of [
    'queryMaxResultRows',
    'queryMaxResultBytes',
    'queryCursorPageSize',
    'schemaSnapshotFreshnessSeconds',
  ] as const) {
    const value = Number(form[key])
    if (!Number.isSafeInteger(value) || value < 1)
      errors[key] = 'Enter a whole number greater than zero.'
  }
  return errors
}

function apiFieldErrors(errors: Record<string, string>): FormErrors {
  return {
    queryMaxResultRows: errors.query_max_result_rows,
    queryMaxResultBytes: errors.query_max_result_bytes,
    queryCursorPageSize: errors.query_cursor_page_size,
    schemaSnapshotFreshnessSeconds: errors.schema_snapshot_freshness_seconds,
  }
}

class LocalValidationError extends Error {}
