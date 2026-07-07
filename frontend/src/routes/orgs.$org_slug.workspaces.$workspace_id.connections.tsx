import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { orgEnvironmentsQueryOptions, orgWorkspaceConnectionsQueryOptions } from '#/lib/api/query'
import type { Connection } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '#/components/ui/table'
import { DriverBadge } from '#/components/ide/DriverBadge'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { TableColumnHeader } from '#/components/TableColumnHeader'
import { TableEmptyState } from '#/components/EmptyState'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id/connections')({
  component: WorkspaceConnectionsPage,
  pendingComponent: RoutePending,
})

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function WorkspaceConnectionsPage() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize, toggleSort } = useListPageState({
    page: 1,
    page_size: 10,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const connections = useQuery(orgWorkspaceConnectionsQueryOptions(orgSlug, workspaceId, query))
  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspaceId, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const environmentNames = new Map((environments.data?.items ?? []).map((env) => [env.id, env.name]))

  const items = connections.data?.items ?? []
  const page = connections.data?.page ?? Number(query.page ?? 1)
  const pageSize = connections.data?.page_size ?? Number(query.page_size ?? 10)
  const total = connections.data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!connections.error) return
    toast.error(connections.error instanceof Error ? connections.error.message : 'Failed to load connections')
  }, [connections.error])

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-2xl font-semibold tracking-tight">Connections</h1>
            <p className="text-sm text-muted-foreground">
              {!connections.isLoading && total > 0
                ? `${total} connection${total !== 1 ? 's' : ''} in this workspace`
                : 'Databases reachable from this workspace. Create and edit connections in the IDE.'}
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
            <Icon name="database-lightning" size={20} data-icon="inline-start" />
            Open in IDE
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
                  <TableColumnHeader label="Name" sort="name" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Driver" sort="driver" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Environment" />
                </TableHead>
                <TableHead>
                  <TableColumnHeader label="Created" sort="created_at" currentSort={query.sort} currentOrder={query.order} onSortChange={toggleSort} />
                </TableHead>
                <TableHead className="text-end">
                  <TableColumnHeader label="Actions" />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {connections.isLoading ? <ConnectionTableSkeleton /> : null}
              {connections.isError ? <TableEmptyState colSpan={5} icon="flow-connection" message="Failed to load connections." /> : null}
              {!connections.isLoading && !connections.isError && items.length === 0 ? (
                <TableEmptyState
                  colSpan={5}
                  icon="flow-connection"
                  message={query.q ? 'No connections matched your search.' : 'No connections yet'}
                  description={query.q ? undefined : 'Create your first connection from the IDE explorer.'}
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
    </div>
  )
}

function ConnectionRow({
  connection,
  environmentName,
  orgSlug,
  workspaceId,
}: {
  connection: Connection
  environmentName: string | undefined
  orgSlug: string
  workspaceId: string
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
      <TableCell className="text-muted-foreground">{dateFormatter.format(new Date(connection.created_at))}</TableCell>
      <TableCell className="text-end">
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
          <Icon name="database-lightning" size={20} data-icon="inline-start" />
          Open in IDE
        </Button>
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
