import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Navigate, createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { RoutePending } from '#/components/RoutePending'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { Button } from '#/components/ui/button'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import {
  instanceConfigurationQueryOptions,
  instanceSettingsQueryOptions,
  orgQueryOptions,
  orgWorkspacesQueryOptions,
  queryKeys,
} from '#/lib/api/query'
import type { InstanceSettings, Organization } from '#/lib/api/types'
import { useDesktopRuntime } from '#/lib/desktop/context'
import {
  getDesktopInfo,
  createDesktopBackup,
  openDesktopReleasePage,
  revealDesktopBackupDirectory,
  revealDesktopDataDirectory,
  revealDesktopLogDirectory,
  saveDesktopDiagnostics,
  restoreDesktopBackup,
  type DesktopInfo,
} from '#/lib/desktop/runtime'
import { Icon } from '#/lib/icons'
import { usePageTitle } from '#/lib/page-title'
import { CreateWorkspaceDialog } from '#/components/workspaces/CreateWorkspaceDialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { WorkspaceSettingsContent } from './orgs.$org_slug.workspaces.$workspace_id.settings'

export const Route = createFileRoute('/desktop/settings')({ component: DesktopSettingsPage })

interface DesktopSettingsForm {
  schemaSnapshotsEnabled: boolean
  maskConnectionCredentialsOnEdit: boolean
  queryMaxResultRows: string
  queryMaxResultBytes: string
  queryCursorPageSize: string
  schemaSnapshotFreshnessSeconds: string
  exportsSyncMaxBytes: string
  exportsBackgroundMaxBytes: string
  fileRevisionsEnabled: boolean
  fileRevisionsKeepLatest: string
  queryHistoryMode: InstanceSettings['query_history_mode']
  queryHistoryRetentionCount: string
  queryFavoritesMode: InstanceSettings['query_favorites_mode']
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
    exportsSyncMaxBytes: String(settings.exports_sync_max_bytes),
    exportsBackgroundMaxBytes: String(settings.exports_background_max_bytes),
    fileRevisionsEnabled: settings.file_revisions_enabled,
    fileRevisionsKeepLatest: String(settings.file_revisions_keep_latest),
    queryHistoryMode: settings.query_history_mode,
    queryHistoryRetentionCount: String(settings.query_history_retention_count),
    queryFavoritesMode: settings.query_favorites_mode,
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
  const configuration = useQuery(instanceConfigurationQueryOptions())
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
        ['exportsSyncMaxBytes', 'exports_sync_max_bytes'],
        ['exportsBackgroundMaxBytes', 'exports_background_max_bytes'],
        ['fileRevisionsKeepLatest', 'file_revisions_keep_latest'],
        ['queryHistoryRetentionCount', 'query_history_retention_count'],
      ] as const
      for (const [formKey, apiKey] of numericFields) {
        if (form[formKey] !== original[formKey]) instancePatch[apiKey] = Number(form[formKey])
      }
      if (form.fileRevisionsEnabled !== original.fileRevisionsEnabled)
        instancePatch.file_revisions_enabled = form.fileRevisionsEnabled
      if (form.queryHistoryMode !== original.queryHistoryMode)
        instancePatch.query_history_mode = form.queryHistoryMode
      if (form.queryFavoritesMode !== original.queryFavoritesMode)
        instancePatch.query_favorites_mode = form.queryFavoritesMode

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
      <DesktopSettingsFrame orgSlug={orgSlug}>
        <Alert variant="destructive">
          <AlertTitle>Settings unavailable</AlertTitle>
          <AlertDescription>Settings are temporarily unavailable.</AlertDescription>
        </Alert>
      </DesktopSettingsFrame>
    )
  }
  if (org.isError || settings.isError) {
    return (
      <DesktopSettingsFrame orgSlug={orgSlug}>
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
    <DesktopSettingsFrame orgSlug={orgSlug}>
      <Tabs defaultValue="workspaces" className="min-h-0 gap-6">
        <div className="min-w-0 overflow-x-auto overflow-y-hidden border-b border-border">
          <TabsList variant="line" aria-label="Settings sections" className="min-w-max">
            <TabsTrigger value="workspaces">Workspaces</TabsTrigger>
            <TabsTrigger value="data">Data</TabsTrigger>
            <TabsTrigger value="storage">Storage &amp; Recovery</TabsTrigger>
            <TabsTrigger value="diagnostics">Diagnostics</TabsTrigger>
            <TabsTrigger value="about">About</TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="workspaces">
          <DesktopWorkspacesSection orgSlug={orgSlug} />
        </TabsContent>
        <TabsContent value="data" className="space-y-8">
          <SettingsSection
            title="Schema metadata"
            description="Control metadata retained for local editor features."
          >
            <div className="pt-4">
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
            </div>
          </SettingsSection>
          <SettingsSection
            title="Query limits"
            description="Bound local query results and schema refresh behavior."
            action={
              <Button onClick={() => save.mutate()} disabled={!changed || save.isPending}>
                {save.isPending ? 'Saving…' : 'Save changes'}
              </Button>
            }
          >
            <div className="pt-4">
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
            </div>
          </SettingsSection>
          <SettingsSection
            title="History, files & exports"
            description="Choose what is retained locally and how large exported results may be."
          >
            <div className="pt-4">
              <FieldGroup className="grid gap-4 sm:grid-cols-2">
                <NumberField
                  label="Synchronous export bytes"
                  value={form.exportsSyncMaxBytes}
                  error={fieldErrors.exportsSyncMaxBytes}
                  onChange={(value) => update('exportsSyncMaxBytes', value)}
                />
                <NumberField
                  label="Background export bytes"
                  value={form.exportsBackgroundMaxBytes}
                  error={fieldErrors.exportsBackgroundMaxBytes}
                  onChange={(value) => update('exportsBackgroundMaxBytes', value)}
                />
                <NumberField
                  label="File revisions retained"
                  value={form.fileRevisionsKeepLatest}
                  error={fieldErrors.fileRevisionsKeepLatest}
                  onChange={(value) => update('fileRevisionsKeepLatest', value)}
                />
                <NumberField
                  label="Query history entries retained"
                  value={form.queryHistoryRetentionCount}
                  error={fieldErrors.queryHistoryRetentionCount}
                  onChange={(value) => update('queryHistoryRetentionCount', value)}
                />
                <ChoiceField
                  label="Query history"
                  value={form.queryHistoryMode}
                  options={['backend', 'local', 'off']}
                  onChange={(value) =>
                    update('queryHistoryMode', value as typeof form.queryHistoryMode)
                  }
                />
                <ChoiceField
                  label="Query favorites"
                  value={form.queryFavoritesMode}
                  options={['backend', 'local', 'off']}
                  onChange={(value) =>
                    update('queryFavoritesMode', value as typeof form.queryFavoritesMode)
                  }
                />
              </FieldGroup>
              <div className="mt-4">
                <CheckboxField
                  id="file-revisions"
                  checked={form.fileRevisionsEnabled}
                  title="Keep file revisions"
                  description="Retain prior versions of saved SQL files for local recovery."
                  onChange={(checked) => update('fileRevisionsEnabled', checked)}
                />
              </div>
            </div>
          </SettingsSection>
        </TabsContent>
        <TabsContent value="storage" className="space-y-8">
          <SettingsSection
            title="Local storage"
            description="SQLWarden keeps application data in platform-specific local folders."
          >
            <div className="divide-y divide-border pt-3">
              <PathRow
                label="Data directory"
                value={info?.paths.data_dir}
                action="Reveal"
                onAction={() => void revealDirectory(revealDesktopDataDirectory, 'data directory')}
              />
              <PathRow label="Database" value={info?.paths.database} />
              <PathRow label="Files" value={info?.paths.files} />
              <PathRow
                label="Credential storage"
                value={
                  info?.secret_store === 'keyring'
                    ? 'Operating system credential store'
                    : info?.secret_store === 'protected-file'
                      ? 'Protected local file (fallback)'
                      : 'Unavailable'
                }
              />
              <PathRow
                label="Backup directory"
                value={info?.paths.backups}
                action="Reveal"
                onAction={() =>
                  void revealDirectory(revealDesktopBackupDirectory, 'backup directory')
                }
              />
            </div>
          </SettingsSection>
          <SettingsSection
            title="Backup & recovery"
            description="Create a portable backup or restore one after validating its contents."
          >
            <div className="flex flex-wrap gap-2 pt-4">
              <Button
                variant="outline"
                onClick={() =>
                  void createDesktopBackup()
                    .then((path) => {
                      if (path) toast.success('Backup created')
                    })
                    .catch((error) => toast.error(errorMessage(error, 'Failed to create backup')))
                }
              >
                <Icon name="download-01" size={18} /> Create backup
              </Button>
              <Button
                variant="outline"
                onClick={() =>
                  void restoreDesktopBackup().catch((error) =>
                    toast.error(errorMessage(error, 'Failed to restore backup')),
                  )
                }
              >
                <Icon name="refresh" size={18} /> Restore backup
              </Button>
            </div>
          </SettingsSection>
        </TabsContent>
        <TabsContent value="diagnostics" className="space-y-8">
          <SettingsSection
            title="Runtime"
            description="Details useful when diagnosing this local installation."
          >
            <div className="divide-y divide-border pt-3">
              <PathRow label="Mode" value={configuration.data?.mode} />
              <PathRow label="Database driver" value={configuration.data?.database_driver} />
              <PathRow label="File storage" value={configuration.data?.file_storage_mode} />
              <PathRow
                label="Restart required"
                value={configuration.data?.restart_required ? 'Yes' : 'No'}
              />
              <PathRow
                label="Logs"
                value={info?.paths.logs}
                action="Reveal"
                onAction={() => void revealDirectory(revealDesktopLogDirectory, 'log directory')}
              />
            </div>
          </SettingsSection>
          {info?.startup_error ? (
            <Alert variant="destructive">
              <AlertTitle>Startup warning</AlertTitle>
              <AlertDescription>{info.startup_error}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={() =>
                void saveDesktopDiagnostics().catch((error) =>
                  toast.error(errorMessage(error, 'Failed to save diagnostics')),
                )
              }
            >
              <Icon name="download-01" size={18} /> Save diagnostics
            </Button>
          </div>
        </TabsContent>
        <TabsContent value="about">
          <SettingsSection
            title="SQLWarden Desktop"
            description="A local-first SQL workspace powered by the shared SQLWarden server core."
          >
            <div className="divide-y divide-border pt-3">
              <PathRow label="Version" value={info?.version ?? 'Unknown'} />
              <PathRow label="Configuration" value={info?.paths.config_file} />
            </div>
            <div className="pt-4">
              <Button variant="outline" onClick={() => void openDesktopReleasePage()}>
                <Icon name="arrow-up-right-01" size={18} /> Check for updates
              </Button>
            </div>
          </SettingsSection>
        </TabsContent>
      </Tabs>
    </DesktopSettingsFrame>
  )
}

function SettingsSection({
  title,
  description,
  action,
  children,
}: {
  title: string
  description: string
  action?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="border-t border-border pt-5 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-heading text-base font-semibold tracking-tight">{title}</h3>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}

function DesktopWorkspacesSection({ orgSlug }: { orgSlug: string }) {
  const workspaces = useQuery(
    orgWorkspacesQueryOptions(orgSlug, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const [creating, setCreating] = useState(false)
  const [managingWorkspaceId, setManagingWorkspaceId] = useState<string>()
  const items = workspaces.data?.items ?? []

  return (
    <SettingsSection
      title="Workspaces"
      description="Organize connections and saved queries into separate local work areas."
      action={
        <Button onClick={() => setCreating(true)}>
          <Icon name="plus-sign" size={18} /> New workspace
        </Button>
      }
    >
      <div className="mt-4 divide-y divide-border border-y border-border">
        {workspaces.isLoading ? (
          <p className="py-6 text-sm text-muted-foreground">Loading workspaces…</p>
        ) : null}
        {workspaces.isError ? (
          <div className="flex items-center justify-between gap-3 py-4">
            <p className="text-sm text-muted-foreground">Couldn&apos;t load workspaces.</p>
            <Button variant="outline" size="sm" onClick={() => void workspaces.refetch()}>
              Retry
            </Button>
          </div>
        ) : null}
        {!workspaces.isLoading && !workspaces.isError && items.length === 0 ? (
          <p className="py-6 text-sm text-muted-foreground">
            No workspaces yet. Create one to start connecting to databases.
          </p>
        ) : null}
        {items.map((workspace) => (
          <div key={workspace.id} className="flex flex-wrap items-center gap-3 py-3">
            <Icon name="briefcase-01" size={17} className="text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{workspace.name}</p>
              <p className="truncate text-xs text-muted-foreground">
                {workspace.description || 'No description'}
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setManagingWorkspaceId(String(workspace.id))}
            >
              Manage
            </Button>
            <Button
              variant="outline"
              size="sm"
              nativeButton={false}
              render={
                <Link
                  to="/orgs/$org_slug/workspaces/$workspace_id/ide"
                  params={{ org_slug: orgSlug, workspace_id: String(workspace.id) }}
                />
              }
            >
              Open
            </Button>
          </div>
        ))}
      </div>
      {creating ? (
        <CreateWorkspaceDialog
          orgSlug={orgSlug}
          open={creating}
          onOpenChange={setCreating}
          onCreated={() => undefined}
        />
      ) : null}
      <Dialog
        open={Boolean(managingWorkspaceId)}
        onOpenChange={(open) => {
          if (!open) setManagingWorkspaceId(undefined)
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>Manage workspace</DialogTitle>
            <DialogDescription>Update this local workspace or remove it.</DialogDescription>
          </DialogHeader>
          {managingWorkspaceId ? (
            <WorkspaceSettingsContent
              orgSlug={orgSlug}
              workspaceId={managingWorkspaceId}
              onDeleted={() => setManagingWorkspaceId(undefined)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </SettingsSection>
  )
}

function DesktopSettingsFrame({
  orgSlug,
  children,
}: {
  orgSlug: string
  children: React.ReactNode
}) {
  return (
    <main className="min-h-svh bg-background">
      <header className="sticky top-0 z-10 flex h-14 items-center border-b border-border bg-background px-4 md:px-6">
        <div className="mx-auto flex w-full max-w-5xl items-center gap-3">
          <Button
            variant="ghost"
            size="icon-sm"
            nativeButton={false}
            render={<Link to="/ide/$org_slug" params={{ org_slug: orgSlug }} />}
            aria-label="Back to editor"
          >
            <Icon name="arrow-left-01" size={18} />
          </Button>
          <h1 className="font-heading text-base font-semibold tracking-tight">Settings</h1>
        </div>
      </header>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-5 px-4 py-6 md:px-6">
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

function ChoiceField({
  label,
  value,
  options,
  labels,
  onChange,
}: {
  label: string
  value: string
  options: readonly string[]
  labels?: Record<string, string>
  onChange: (value: string) => void
}) {
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <Select value={value} onValueChange={(next) => next && onChange(next)}>
        <SelectTrigger aria-label={label} className="w-full">
          <SelectValue>{labels?.[value] ?? value}</SelectValue>
        </SelectTrigger>
        <SelectContent align="start">
          <SelectGroup>
            {options.map((option) => (
              <SelectItem key={option} value={option}>
                {labels?.[option] ?? option}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
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
    'exportsSyncMaxBytes',
    'queryHistoryRetentionCount',
  ] as const) {
    const value = Number(form[key])
    if (!Number.isSafeInteger(value) || value < 1)
      errors[key] = 'Enter a whole number greater than zero.'
  }
  for (const key of ['exportsBackgroundMaxBytes', 'fileRevisionsKeepLatest'] as const) {
    const value = Number(form[key])
    if (!Number.isSafeInteger(value) || value < 0)
      errors[key] = 'Enter a whole number of zero or greater.'
  }
  return errors
}

function apiFieldErrors(errors: Record<string, string>): FormErrors {
  return {
    queryMaxResultRows: errors.query_max_result_rows,
    queryMaxResultBytes: errors.query_max_result_bytes,
    queryCursorPageSize: errors.query_cursor_page_size,
    schemaSnapshotFreshnessSeconds: errors.schema_snapshot_freshness_seconds,
    exportsSyncMaxBytes: errors.exports_sync_max_bytes,
    exportsBackgroundMaxBytes: errors.exports_background_max_bytes,
    fileRevisionsKeepLatest: errors.file_revisions_keep_latest,
    queryHistoryRetentionCount: errors.query_history_retention_count,
  }
}

class LocalValidationError extends Error {}
