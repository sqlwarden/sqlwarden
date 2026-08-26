import { errorMessage } from '#/lib/api/errors'
import { useOrganizationPageTitle } from '#/lib/page-title'
import { formatDate } from '#/lib/format'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgQueryOptions, queryKeys } from '#/lib/api/query'
import type { Organization } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '#/components/ui/alert-dialog'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { RoutePending } from '#/components/RoutePending'
import {
  OrganizationRuntimeSettingsPanel,
  type OrganizationSettingsActionState,
} from './orgs.$org_slug.settings.runtime'

type OrganizationSettingsTab = 'general' | 'runtime' | 'danger'

function organizationSettingsSearch(search: Record<string, unknown>): {
  tab?: OrganizationSettingsTab
} {
  return search.tab === 'runtime' || search.tab === 'danger' || search.tab === 'general'
    ? { tab: search.tab }
    : {}
}

export const Route = createFileRoute('/orgs/$org_slug/settings/general')({
  component: OrganizationGeneralSettingsPage,
  pendingComponent: RoutePending,
  validateSearch: organizationSettingsSearch,
})

type OrgFieldErrors = {
  name?: string
}

function OrganizationGeneralSettingsPage() {
  useOrganizationPageTitle('General')
  const { org_slug: orgSlug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const org = useQuery(orgQueryOptions(orgSlug))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const [name, setName] = useState('')
  const [schemaSnapshotsEnabled, setSchemaSnapshotsEnabled] = useState(true)
  const [maskConnectionCredentialsOnEdit, setMaskConnectionCredentialsOnEdit] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<OrgFieldErrors>({})
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [activeSection, setActiveSection] = useState<OrganizationSettingsTab>(
    search.tab ?? 'general',
  )
  const [runtimeAction, setRuntimeAction] = useState<OrganizationSettingsActionState>({
    disabled: true,
    hasChanges: false,
    pending: false,
  })

  const permissions = effectivePermissions.data?.permissions
  const canReadRuntime = hasPermission(permissions, permission.orgRead)
  const canWrite = hasPermission(permissions, permission.orgWrite)
  const canDelete = hasPermission(permissions, permission.orgDelete)

  useEffect(() => {
    const requested = search.tab ?? 'general'
    setActiveSection(requested === 'runtime' && !canReadRuntime ? 'general' : requested)
  }, [canReadRuntime, search.tab])

  useEffect(() => {
    if (!org.data) return
    setName(org.data.name)
    setSchemaSnapshotsEnabled(org.data.schema_snapshots_enabled ?? true)
    setMaskConnectionCredentialsOnEdit(org.data.mask_connection_credentials_on_edit ?? false)
  }, [org.data])

  const updateOrg = useMutation({
    mutationFn: async () =>
      api.patch<Organization>(`/api/v1/orgs/${orgSlug}`, {
        name: name.trim(),
        schema_snapshots_enabled: schemaSnapshotsEnabled,
        mask_connection_credentials_on_edit: maskConnectionCredentialsOnEdit,
      }),
    onSuccess: async (updated) => {
      setFieldErrors({})
      toast.success('Organization updated')
      queryClient.setQueryData(queryKeys.org(orgSlug), updated)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.accountOrganizationsScope() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.instanceOrganizationsScope() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors({
          name: error.fieldErrors.name,
        })
        if (error.fieldErrors.name) return
      }
      toast.error(errorMessage(error, 'Failed to update organization'))
    },
  })

  const deleteOrg = useMutation({
    mutationFn: async () => api.delete<void>(`/api/v1/orgs/${orgSlug}`),
    onSuccess: async () => {
      toast.success('Organization deleted')
      queryClient.removeQueries({ queryKey: queryKeys.org(orgSlug) })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.accountOrganizationsScope() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
      await navigate({ to: '/', replace: true })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to delete organization'))
    },
  })

  if (org.isLoading) {
    return <RoutePending />
  }

  if (org.isError || !org.data) {
    return (
      <div className="flex flex-col gap-2">
        <h1 className="font-heading text-2xl font-semibold tracking-tight">General</h1>
        <p className="text-sm text-muted-foreground">Failed to load organization settings.</p>
      </div>
    )
  }

  const hasChanges =
    name.trim() !== org.data.name ||
    schemaSnapshotsEnabled !== (org.data.schema_snapshots_enabled ?? true) ||
    maskConnectionCredentialsOnEdit !== (org.data.mask_connection_credentials_on_edit ?? false)
  const deleteMatches = deleteConfirmation === org.data.slug
  const generalAction: OrganizationSettingsActionState = {
    disabled: !canWrite,
    hasChanges,
    pending: updateOrg.isPending,
  }
  const fallbackAction = hasChanges
    ? { state: generalAction, formId: 'organization-general-form' }
    : runtimeAction.hasChanges
      ? { state: runtimeAction, formId: 'organization-runtime-form' }
      : undefined
  const activeAction =
    activeSection === 'general'
      ? { state: generalAction, formId: 'organization-general-form' }
      : activeSection === 'runtime'
        ? { state: runtimeAction, formId: 'organization-runtime-form' }
        : fallbackAction

  function submitGeneral(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!name.trim()) {
      setFieldErrors({ name: 'Name is required.' })
      return
    }

    setFieldErrors({})
    void updateOrg.mutateAsync().catch(() => {})
  }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <h1 className="font-heading text-2xl font-semibold tracking-tight">General</h1>
        <p className="text-sm text-muted-foreground">
          Manage organization identity, data boundaries, storage behavior, and lifecycle actions.
        </p>
      </div>

      <Tabs
        value={activeSection}
        onValueChange={(value) => setActiveSection(value as OrganizationSettingsTab)}
        className="gap-6"
      >
        <div className="flex flex-col gap-3 border-b border-border sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0 overflow-x-auto overflow-y-hidden">
            <TabsList variant="line" className="min-w-max">
              <TabsTrigger value="general">Organization</TabsTrigger>
              {canReadRuntime ? <TabsTrigger value="runtime">Data Controls</TabsTrigger> : null}
              <TabsTrigger value="danger">Danger Zone</TabsTrigger>
            </TabsList>
          </div>

          {activeAction ? (
            <div
              aria-label="Organization settings actions"
              className="flex shrink-0 items-center justify-end gap-3 pb-2 sm:pb-1"
              role="region"
            >
              {activeAction.state.hasChanges ? (
                <p aria-live="polite" className="text-xs text-muted-foreground" role="status">
                  You have unsaved changes.
                </p>
              ) : null}
              <Button
                type="submit"
                form={activeAction.formId}
                disabled={
                  activeAction.state.disabled ||
                  !activeAction.state.hasChanges ||
                  activeAction.state.pending
                }
              >
                {activeAction.state.pending ? 'Saving...' : 'Save changes'}
              </Button>
            </div>
          ) : null}
        </div>

        <TabsContent value="general">
          <form
            id="organization-general-form"
            className="flex flex-col gap-6"
            onSubmit={submitGeneral}
          >
            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Organization Details</CardTitle>
                <CardDescription>
                  Manage the organization identity and reference information.
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-5">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Organization name" error={fieldErrors.name}>
                    <Input
                      aria-label="Organization name"
                      value={name}
                      disabled={!canWrite || updateOrg.isPending}
                      aria-invalid={fieldErrors.name ? true : undefined}
                      onChange={(event) => {
                        setName(event.target.value)
                        setFieldErrors((current) => ({ ...current, name: undefined }))
                      }}
                    />
                  </Field>
                  <Field label="Slug">
                    <Input aria-label="Slug" value={org.data.slug} disabled />
                    <p className="text-xs text-muted-foreground">
                      Slug changes are disabled because they affect URLs and integrations.
                    </p>
                  </Field>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Organization ID">
                    <Input aria-label="Organization ID" value={org.data.id} disabled />
                  </Field>
                  <Field label="Created">
                    <Input aria-label="Created" value={formatDate(org.data.created_at)} disabled />
                  </Field>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Schema Metadata</CardTitle>
                <CardDescription>
                  Control whether database structure metadata is retained between sessions.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <label className="flex cursor-pointer items-start gap-3 py-2">
                  <Checkbox
                    checked={schemaSnapshotsEnabled}
                    disabled={!canWrite || updateOrg.isPending}
                    onCheckedChange={(checked) => setSchemaSnapshotsEnabled(checked === true)}
                  />
                  <span className="flex flex-col gap-1">
                    <span className="font-medium text-foreground">Persist schema snapshots</span>
                    <span className="text-muted-foreground">
                      Store database structure metadata for disconnected schema browsing and future
                      autocomplete. Disabling this deletes stored snapshots and keeps metadata only
                      in memory while a connection session is active.
                    </span>
                  </span>
                </label>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b border-border">
                <CardTitle>Connection Security</CardTitle>
                <CardDescription>
                  Set the default credential handling behavior for connection forms.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <label className="flex cursor-pointer items-start gap-3 py-2">
                  <Checkbox
                    checked={maskConnectionCredentialsOnEdit}
                    disabled={!canWrite || updateOrg.isPending}
                    onCheckedChange={(checked) =>
                      setMaskConnectionCredentialsOnEdit(checked === true)
                    }
                  />
                  <span className="flex flex-col gap-1">
                    <span className="font-medium text-foreground">
                      Mask connection credentials on edit
                    </span>
                    <span className="text-muted-foreground">
                      Never pre-fill saved passwords when editing connections. Members must re-enter
                      credentials to change them.
                    </span>
                  </span>
                </label>
              </CardContent>
            </Card>

            {!canWrite ? (
              <p className="text-xs text-muted-foreground">
                You need organization write permission to change these settings.
              </p>
            ) : null}
          </form>
        </TabsContent>

        {canReadRuntime ? (
          <TabsContent value="runtime">
            <OrganizationRuntimeSettingsPanel
              orgSlug={orgSlug}
              formId="organization-runtime-form"
              onActionStateChange={setRuntimeAction}
            />
          </TabsContent>
        ) : null}

        <TabsContent value="danger">
          <Card>
            <CardHeader className="border-b border-border">
              <CardTitle>Danger Zone</CardTitle>
              <CardDescription>
                Delete this organization and all organization-owned resources.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex flex-col gap-1">
                  <div className="text-sm font-medium">Delete organization</div>
                  <p className="text-muted-foreground">
                    This permanently deletes workspaces, environments, connections, teams, roles,
                    and policy bindings in this organization.
                  </p>
                  {!canDelete ? (
                    <p className="text-xs text-muted-foreground">
                      You need organization delete permission to perform this action.
                    </p>
                  ) : null}
                </div>

                <AlertDialog>
                  <AlertDialogTrigger
                    render={
                      <Button variant="destructive" disabled={!canDelete || deleteOrg.isPending} />
                    }
                  >
                    Delete organization
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Delete organization?</AlertDialogTitle>
                      <AlertDialogDescription>
                        Type <span className="font-medium text-foreground">{org.data.slug}</span> to
                        confirm deletion.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <div className="flex flex-col gap-2">
                      <Label>Organization slug</Label>
                      <Input
                        aria-label="Organization slug"
                        value={deleteConfirmation}
                        disabled={deleteOrg.isPending}
                        onChange={(event) => setDeleteConfirmation(event.target.value)}
                      />
                    </div>
                    <AlertDialogFooter>
                      <AlertDialogCancel
                        variant="ghost"
                        disabled={deleteOrg.isPending}
                        onClick={() => setDeleteConfirmation('')}
                      >
                        Cancel
                      </AlertDialogCancel>
                      <AlertDialogAction
                        variant="destructive"
                        disabled={!deleteMatches || deleteOrg.isPending}
                        onClick={() => {
                          void deleteOrg.mutateAsync().catch(() => {})
                        }}
                      >
                        {deleteOrg.isPending ? 'Deleting...' : 'Delete organization'}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
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
