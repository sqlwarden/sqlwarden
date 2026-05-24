import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { orgEffectivePermissionsQueryOptions, orgQueryOptions, queryKeys } from '#/lib/api/query'
import type { Organization } from '#/lib/api/types'
import { hasPermission, permission } from '#/lib/permissions'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '#/components/ui/alert-dialog'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/orgs/$org_slug/settings/general')({
  component: OrganizationGeneralSettingsPage,
  pendingComponent: RoutePending,
})

type OrgFieldErrors = {
  name?: string
}

function OrganizationGeneralSettingsPage() {
  const { org_slug: orgSlug } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const org = useQuery(orgQueryOptions(orgSlug))
  const effectivePermissions = useQuery(orgEffectivePermissionsQueryOptions(orgSlug, 'org'))
  const [name, setName] = useState('')
  const [fieldErrors, setFieldErrors] = useState<OrgFieldErrors>({})
  const [deleteConfirmation, setDeleteConfirmation] = useState('')

  const permissions = effectivePermissions.data?.permissions
  const canWrite = hasPermission(permissions, permission.orgWrite)
  const canDelete = hasPermission(permissions, permission.orgDelete)

  useEffect(() => {
    if (!org.data) return
    setName(org.data.name)
  }, [org.data])

  const updateOrg = useMutation({
    mutationFn: async () =>
      api.patch<Organization>(`/api/v1/orgs/${orgSlug}`, {
        name: name.trim(),
      }),
    onSuccess: async (updated) => {
      setFieldErrors({})
      toast.success('Organization updated')
      queryClient.setQueryData(queryKeys.org(orgSlug), updated)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['account-organizations'] }),
        queryClient.invalidateQueries({ queryKey: ['instance-organizations'] }),
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
      toast.error(error instanceof Error ? error.message : 'Failed to update organization')
    },
  })

  const deleteOrg = useMutation({
    mutationFn: async () => api.delete<void>(`/api/v1/orgs/${orgSlug}`),
    onSuccess: async () => {
      toast.success('Organization deleted')
      queryClient.removeQueries({ queryKey: queryKeys.org(orgSlug) })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['account-organizations'] }),
        queryClient.invalidateQueries({ queryKey: queryKeys.session() }),
      ])
      await navigate({ to: '/', replace: true })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete organization')
    },
  })

  if (org.isLoading) {
    return <RoutePending />
  }

  if (org.isError || !org.data) {
    return (
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">General</h1>
        <p className="text-sm text-muted-foreground">Failed to load organization settings.</p>
      </div>
    )
  }

  const hasChanges = name.trim() !== org.data.name
  const deleteMatches = deleteConfirmation === org.data.slug

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
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-2xl font-semibold tracking-tight">General</h1>
        <p className="text-sm text-muted-foreground">Manage organization identity and lifecycle actions.</p>
      </div>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Organization Details</CardTitle>
          <CardDescription>Update the organization name shown across SQLWarden.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submitGeneral}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Organization name" error={fieldErrors.name}>
                <Input
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
                <Input value={org.data.slug} disabled />
                <p className="text-xs text-muted-foreground">Slug changes are intentionally disabled because they affect URLs and integrations.</p>
              </Field>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Organization ID">
                <Input value={org.data.id} disabled />
              </Field>
              <Field label="Created">
                <Input value={formatDate(org.data.created_at)} disabled />
              </Field>
            </div>

            {!canWrite ? (
              <p className="text-xs text-muted-foreground">You need organization write permission to change these settings.</p>
            ) : null}

            <div className="flex justify-end">
              <Button type="submit" disabled={!canWrite || !hasChanges || updateOrg.isPending}>
                {updateOrg.isPending ? 'Saving...' : 'Save changes'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Danger Zone</CardTitle>
          <CardDescription>Delete this organization and all org-owned resources.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-col gap-1">
              <div className="text-sm font-medium">Delete organization</div>
              <p className="text-muted-foreground">
                This permanently deletes workspaces, environments, connections, teams, roles, and policy bindings in this organization.
              </p>
              {!canDelete ? (
                <p className="text-xs text-muted-foreground">You need organization delete permission to perform this action.</p>
              ) : null}
            </div>

            <AlertDialog>
              <AlertDialogTrigger render={<Button variant="destructive" disabled={!canDelete || deleteOrg.isPending} />}>
                Delete organization
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete organization?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Type <span className="font-medium text-foreground">{org.data.slug}</span> to confirm deletion.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="flex flex-col gap-2">
                  <Label>Organization slug</Label>
                  <Input
                    value={deleteConfirmation}
                    disabled={deleteOrg.isPending}
                    onChange={(event) => setDeleteConfirmation(event.target.value)}
                  />
                </div>
                <AlertDialogFooter>
                  <AlertDialogCancel variant="ghost" disabled={deleteOrg.isPending} onClick={() => setDeleteConfirmation('')}>
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

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(date)
}
