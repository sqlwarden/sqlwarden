import { errorMessage } from '#/lib/api/errors'
import { formatDate } from '#/lib/format'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { useListPageState } from '#/hooks/use-list-page-state'
import { queryKeys } from '#/lib/api/query-keys'
import {
  orgEffectivePermissionsQueryOptions,
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import type { Connection } from '#/lib/api/types'
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
import { Card, CardContent } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import { DriverBadge } from '#/components/ide/DriverBadge'
import { EditConnectionDialog } from '#/components/ide/EditConnectionDialog'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/connections')({
  component: WorkspaceConnectionsPage,
  pendingComponent: RoutePending,
})

function WorkspaceConnectionsPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const queryClient = useQueryClient()
  const [editingConnection, setEditingConnection] = useState<Connection | null>(null)
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } =
    useListPageState({
      page: 1,
      page_size: 10,
      sort: 'name',
      order: 'asc',
      q: '',
    })

  const effectivePermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId),
  )
  const canEditConnection = hasPermission(
    effectivePermissions.data?.permissions,
    permission.connUpdate,
  )
  const canDeleteConnection = hasPermission(
    effectivePermissions.data?.permissions,
    permission.connDelete,
  )
  const canManageConnection = canEditConnection || canDeleteConnection

  const connections = useQuery(orgWorkspaceConnectionsQueryOptions(orgSlug, workspaceId, query))
  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspaceId, {
      page_size: 100,
      sort: 'name',
      order: 'asc',
    }),
  )
  const environmentNames = new Map(
    (environments.data?.items ?? []).map((env) => [env.id, env.name]),
  )

  const items = connections.data?.items ?? []
  const page = connections.data?.page ?? Number(query.page ?? 1)
  const pageSize = connections.data?.page_size ?? Number(query.page_size ?? 10)
  const total = connections.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!connections.error) return
    toast.error(errorMessage(connections.error, 'Failed to load connections'))
  }, [connections.error])

  const deleteConnection = useMutation({
    mutationFn: async (connectionId: number) =>
      api.delete<void>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${connectionId}`,
      ),
    onSuccess: async () => {
      toast.success('Connection deleted')
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, workspaceId),
      })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to delete connection'))
    },
  })

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="font-heading text-2xl font-semibold tracking-tight">Connections</h1>
            <p className="text-sm text-muted-foreground">
              {!connections.isLoading && total > 0
                ? `${total} connection${total !== 1 ? 's' : ''} in this workspace`
                : 'Databases reachable from this workspace. Create and edit connections in the editor.'}
            </p>
          </div>
          <Button
            render={
              <Link
                to="/ide/$org_slug"
                params={{ org_slug: orgSlug }}
                search={{ ws: Number(workspaceId) }}
              />
            }
          >
            <Icon name="terminal" size={20} data-icon="inline-start" />
            Open in Editor
          </Button>
        </div>

        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search connections"
        />
      </div>

      <Card>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <TableColumnHeader
                    label="Name"
                    sort="name"
                    currentSort={query.sort}
                    currentOrder={query.order}
                    onSortChange={toggleSort}
                  />
                </TableHead>
                <TableHead>
                  <TableColumnHeader
                    label="Driver"
                    sort="driver"
                    currentSort={query.sort}
                    currentOrder={query.order}
                    onSortChange={toggleSort}
                  />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Environment" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader
                    label="Created"
                    sort="created_at"
                    currentSort={query.sort}
                    currentOrder={query.order}
                    onSortChange={toggleSort}
                  />
                </TableHead>
                <TableHead className="text-end">
                  <TableColumnHeader label="Actions" />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {connections.isLoading ? <ConnectionTableSkeleton /> : null}
              {connections.isError ? (
                <TableEmptyState
                  colSpan={5}
                  icon="flow-connection"
                  message="Failed to load connections."
                />
              ) : null}
              {!connections.isLoading && !connections.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={5}
                  icon="flow-connection"
                  message={query.q ? 'No connections matched your search.' : 'No connections yet'}
                  description={
                    query.q ? undefined : 'Create your first connection from the editor explorer.'
                  }
                />
              ) : null}
              {!connections.isLoading && !connections.isError
                ? items.map((connection) => (
                    <ConnectionRow
                      key={connection.id}
                      connection={connection}
                      environmentName={environmentNames.get(connection.environment_id)}
                      orgSlug={orgSlug}
                      workspaceId={workspaceId}
                      canEditConnection={canEditConnection}
                      canDeleteConnection={canDeleteConnection}
                      isDeleting={deleteConnection.isPending}
                      onEdit={setEditingConnection}
                      onDelete={(connectionId) => deleteConnection.mutate(connectionId)}
                    />
                  ))
                : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!connections.isLoading && !connections.isError && items.length > 0 ? (
        <PaginationFooter
          itemLabel="connections"
          page={page}
          pageCount={pageCount}
          pageSize={pageSize}
          total={total}
          isFetching={connections.isFetching}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      ) : null}

      {canManageConnection ? (
        <EditConnectionDialog
          open={editingConnection !== null}
          onOpenChange={(open) => {
            if (!open) setEditingConnection(null)
          }}
          orgSlug={orgSlug}
          workspaceId={Number(workspaceId)}
          connection={editingConnection ?? undefined}
          canRevealDsn={canEditConnection}
        />
      ) : null}
    </div>
  )
}

function ConnectionRow({
  connection,
  environmentName,
  orgSlug,
  workspaceId,
  canEditConnection,
  canDeleteConnection,
  isDeleting,
  onEdit,
  onDelete,
}: {
  connection: Connection
  environmentName: string | undefined
  orgSlug: string
  workspaceId: string
  canEditConnection: boolean
  canDeleteConnection: boolean
  isDeleting: boolean
  onEdit: (connection: Connection) => void
  onDelete: (connectionId: number) => void
}) {
  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md border border-border">
            <DriverBadge driver={connection.driver} size="sm" />
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium text-foreground">{connection.name}</div>
            <div className="truncate text-muted-foreground">Connection</div>
          </div>
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground capitalize">{connection.driver}</TableCell>
      <TableCell className="text-muted-foreground">
        {environmentName ?? `Environment #${connection.environment_id}`}
      </TableCell>
      <TableCell className="text-muted-foreground">{formatDate(connection.created_at)}</TableCell>
      <TableCell className="text-end">
        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            size="sm"
            render={
              <Link
                to="/ide/$org_slug"
                params={{ org_slug: orgSlug }}
                search={{ ws: Number(workspaceId), conn: connection.id }}
              />
            }
          >
            <Icon name="terminal" size={20} data-icon="inline-start" />
            Open in Editor
          </Button>
          {canEditConnection ? (
            <Button type="button" variant="ghost" size="sm" onClick={() => onEdit(connection)}>
              <Icon name="pencil-edit-02" size={20} data-icon="inline-start" />
              Edit
            </Button>
          ) : null}
          {canDeleteConnection ? (
            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button type="button" variant="destructive" size="sm" disabled={isDeleting} />
                }
              >
                <Icon name="delete-02" size={20} data-icon="inline-start" />
                Delete
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete connection?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This permanently deletes{' '}
                    <span className="font-medium text-foreground">{connection.name}</span> and drops
                    any active sessions.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel variant="ghost" disabled={isDeleting}>
                    Cancel
                  </AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    disabled={isDeleting}
                    onClick={() => onDelete(connection.id)}
                  >
                    {isDeleting ? 'Deleting...' : 'Delete'}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}
        </div>
      </TableCell>
    </TableRow>
  )
}

function ConnectionTableSkeleton() {
  return Array.from({ length: 4 }).map((_, index) => (
    <TableRow key={index}>
      <TableCell>
        <div className="flex items-center gap-3">
          <Skeleton className="size-8 rounded-md" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-20" />
          </div>
        </div>
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-20" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-24" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-24" />
      </TableCell>
      <TableCell className="text-end">
        <Skeleton className="ms-auto h-8 w-24" />
      </TableCell>
    </TableRow>
  ))
}
