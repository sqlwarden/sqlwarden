import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgWorkspaceQueryOptions, queryKeys } from '#/lib/api/query'
import { hasPermission, permission } from '#/lib/permissions'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '#/components/ui/alert-dialog'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Textarea } from '#/components/ui/textarea'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/settings')({
  component: WorkspaceSettingsPage,
  pendingComponent: RoutePending,
})

type WorkspaceFieldErrors = {
  name?: string
  description?: string
}

function WorkspaceSettingsPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const workspace = useQuery(orgWorkspaceQueryOptions(orgSlug, workspaceId))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId))
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [fieldErrors, setFieldErrors] = useState<WorkspaceFieldErrors>({})
  const [deleteConfirmation, setDeleteConfirmation] = useState('')

  const permissions = effectivePermissions.data?.permissions
  const canWrite = hasPermission(permissions, permission.wsWrite)
  const canDelete = hasPermission(permissions, permission.wsDelete)

  useEffect(() => {
    if (!workspace.data) return
    setName(workspace.data.name)
    setDescription(workspace.data.description ?? '')
  }, [workspace.data])

  const updateWorkspace = useMutation({
    mutationFn: async () =>
      api.patch<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}`, {
        name: name.trim(),
        description: description.trim(),
      }),
    onSuccess: async () => {
      setFieldErrors({})
      toast.success('Workspace updated')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspace(orgSlug, workspaceId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspaces(orgSlug) }),
      ])
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setFieldErrors({
          name: error.fieldErrors.name,
          description: error.fieldErrors.description,
        })
        if (error.fieldErrors.name || error.fieldErrors.description) return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to update workspace')
    },
  })

  const deleteWorkspace = useMutation({
    mutationFn: async () => api.delete<void>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}`),
    onSuccess: async () => {
      toast.success('Workspace deleted')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspaces(orgSlug) })
      await navigate({ to: '/orgs/$org_slug/workspaces', params: { org_slug: orgSlug }, replace: true })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete workspace')
    },
  })

  if (workspace.isLoading) {
    return <RoutePending />
  }

  if (workspace.isError || !workspace.data) {
    return (
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">Failed to load workspace settings.</p>
      </div>
    )
  }

  const hasChanges = name.trim() !== workspace.data.name || description.trim() !== (workspace.data.description ?? '')
  const deleteMatches = deleteConfirmation === workspace.data.name

  function submitGeneral(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!name.trim()) {
      setFieldErrors({ name: 'Name is required.' })
      return
    }

    setFieldErrors({})
    void updateWorkspace.mutateAsync().catch(() => { })
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">Manage workspace details and lifecycle actions.</p>
      </div>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>General</CardTitle>
          <CardDescription>Update the workspace name and description shown across the organization.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submitGeneral}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Workspace name" error={fieldErrors.name}>
                <Input
                  value={name}
                  disabled={!canWrite || updateWorkspace.isPending}
                  aria-invalid={fieldErrors.name ? true : undefined}
                  onChange={(event) => {
                    setName(event.target.value)
                    setFieldErrors((current) => ({ ...current, name: undefined }))
                  }}
                />
              </Field>
              <Field label="Workspace ID">
                <Input value={workspace.data.id} disabled />
              </Field>
            </div>

            <Field label="Description" error={fieldErrors.description}>
              <Textarea
                value={description}
                placeholder="Optional workspace description"
                disabled={!canWrite || updateWorkspace.isPending}
                aria-invalid={fieldErrors.description ? true : undefined}
                onChange={(event) => {
                  setDescription(event.target.value)
                  setFieldErrors((current) => ({ ...current, description: undefined }))
                }}
              />
            </Field>

            {!canWrite ? (
              <p className="text-xs text-muted-foreground">You need workspace write permission to change these settings.</p>
            ) : null}

            <div className="flex justify-end">
              <Button type="submit" disabled={!canWrite || !hasChanges || updateWorkspace.isPending}>
                {updateWorkspace.isPending ? 'Saving...' : 'Save changes'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Danger Zone</CardTitle>
          <CardDescription>Delete this workspace and its environments, connections, workspace roles, and policy bindings.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-col gap-1">
              <div className="text-sm font-medium">Delete workspace</div>
              <p className="text-muted-foreground">
                This action is permanent. Existing workspace resources will no longer be available.
              </p>
              {!canDelete ? (
                <p className="text-xs text-muted-foreground">You need workspace delete permission to perform this action.</p>
              ) : null}
            </div>

            <AlertDialog>
              <AlertDialogTrigger render={<Button variant="destructive" disabled={!canDelete || deleteWorkspace.isPending} />}>
                Delete workspace
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete workspace?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Type <span className="font-medium text-foreground">{workspace.data.name}</span> to confirm deletion.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="flex flex-col gap-2">
                  <Label>Workspace name</Label>
                  <Input
                    value={deleteConfirmation}
                    disabled={deleteWorkspace.isPending}
                    onChange={(event) => setDeleteConfirmation(event.target.value)}
                  />
                </div>
                <AlertDialogFooter>
                  <AlertDialogCancel variant="ghost" disabled={deleteWorkspace.isPending} onClick={() => setDeleteConfirmation('')}>
                    Cancel
                  </AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    disabled={!deleteMatches || deleteWorkspace.isPending}
                    onClick={() => {
                      void deleteWorkspace.mutateAsync().catch(() => { })
                    }}
                  >
                    {deleteWorkspace.isPending ? 'Deleting...' : 'Delete workspace'}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </CardContent>
      </Card>
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
