import { errorMessage } from '#/lib/api/errors'
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { accountOrganizationsQueryOptions } from '#/lib/api/query'
import { Badge } from '#/components/ui/badge'
import { Card, CardContent } from '#/components/ui/card'
import { EmptyState } from '#/components/EmptyState'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'
import { Skeleton } from '#/components/ui/skeleton'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/settings/my-organizations')({
  component: SettingsMyOrganizationsPage,
  pendingComponent: RoutePending,
})

function SettingsMyOrganizationsPage() {
  usePageTitle('My Organizations', 'Settings')
  const { query, searchText, setSearchText, clearSearch, setPage, setPageSize } = useListPageState({
    page: 1,
    page_size: 12,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  const organizations = useQuery(accountOrganizationsQueryOptions(query))
  const data = organizations.data
  const items = data?.items ?? []
  const page = data?.page ?? Number(query.page ?? 1)
  const pageSize = data?.page_size ?? Number(query.page_size ?? 12)
  const total = data?.total ?? 0
  const pageCount = total > 0 ? Math.ceil(total / pageSize) : 1

  useEffect(() => {
    if (!organizations.error) {
      return
    }

    toast.error(errorMessage(organizations.error, 'Failed to load organizations'))
  }, [organizations.error])

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <h2 className="font-heading text-2xl font-semibold tracking-tight">My Organizations</h2>
          <p className="text-sm text-muted-foreground">
            {!organizations.isLoading && total > 0
              ? `${total} organization${total !== 1 ? 's' : ''} you belong to`
              : 'Choose an organization to continue.'}
          </p>
        </div>

        <SearchInput
          value={searchText}
          onValueChange={setSearchText}
          onClear={clearSearch}
          placeholder="Search organizations"
        />
      </div>

      {organizations.isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="flex flex-col rounded-lg border border-border bg-card">
              <div className="flex flex-col gap-3 p-5">
                <div className="flex items-start gap-3">
                  <Skeleton className="size-9 shrink-0 rounded-md" />
                  <div className="flex flex-1 flex-col gap-2 pt-1">
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-4 w-28" />
                      <Skeleton className="h-4 w-12" />
                    </div>
                    <Skeleton className="h-3 w-20" />
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-4 border-t border-border/60 px-5 py-3">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-3 w-16" />
              </div>
            </div>
          ))}
        </div>
      ) : null}

      {organizations.isError ? (
        <Card>
          <CardContent>
            <EmptyState
              icon="building-04"
              message="Failed to load organizations"
              description="Refresh the page and try again."
            />
          </CardContent>
        </Card>
      ) : null}

      {!organizations.isLoading && !organizations.isError && items.length === 0 ? (
        <Card>
          <CardContent>
            <EmptyState
              icon="building-04"
              message={query.q ? 'No organizations matched your search.' : 'No organizations found'}
              description={
                query.q
                  ? 'Try a different name or slug.'
                  : 'You do not belong to any organizations yet.'
              }
            />
          </CardContent>
        </Card>
      ) : null}

      {!organizations.isLoading && !organizations.isError && items.length > 0 ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {items.map((organization) => (
              <Link
                key={organization.id}
                to="/orgs/$org_slug/workspaces"
                params={{ org_slug: organization.slug }}
                className="group relative flex flex-col overflow-hidden rounded-lg border border-border bg-card text-card-foreground transition-all hover:border-foreground/20 hover:shadow-sm"
              >
                <span className="absolute inset-y-0 left-0 w-0.5 scale-y-0 bg-primary transition-transform group-hover:scale-y-100" />
                <div className="flex flex-1 items-start gap-3 p-5">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground transition-colors group-hover:text-primary">
                    <Icon name="building-04" size={18} />
                  </div>
                  <div className="min-w-0 flex-1 pt-0.5">
                    <div className="flex items-center gap-2">
                      <p className="truncate font-semibold leading-tight tracking-tight transition-colors group-hover:text-primary">
                        {organization.name}
                      </p>
                      <Badge
                        variant="outline"
                        className="shrink-0 capitalize text-[10px] px-1.5 h-4 py-0"
                      >
                        {organization.role}
                      </Badge>
                    </div>
                    <p className="mt-1.5 text-xs text-muted-foreground">@{organization.slug}</p>
                  </div>
                </div>
                <div className="flex items-center gap-4 border-t border-border/60 px-5 py-3 text-xs text-muted-foreground">
                  <div className="flex items-center gap-1.5">
                    <Icon name="user-multiple" size={14} />
                    <span className="tabular-nums">{organization.member_count}</span>
                    <span>{organization.member_count === 1 ? 'member' : 'members'}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Icon name="user-group" size={14} />
                    <span className="tabular-nums">{organization.team_count}</span>
                    <span>{organization.team_count === 1 ? 'team' : 'teams'}</span>
                  </div>
                </div>
              </Link>
            ))}
          </div>

          <PaginationFooter
            itemLabel="organizations"
            page={page}
            pageCount={pageCount}
            pageSize={pageSize}
            total={total}
            isFetching={organizations.isFetching}
            pageSizeOptions={[12, 24, 48, 96]}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        </>
      ) : null}
    </div>
  )
}
