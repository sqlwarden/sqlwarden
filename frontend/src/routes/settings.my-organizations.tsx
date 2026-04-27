import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { Building04Icon } from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import { useListPageState } from '#/hooks/use-list-page-state'
import { accountOrganizationsQueryOptions } from '#/lib/api/query'
import { Button } from '#/components/ui/button'
import { Badge } from '#/components/ui/badge'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle, CardAction } from '#/components/ui/card'
import { EmptyState } from '#/components/EmptyState'
import { getInitials } from '#/components/InitialsAvatar'
import { PaginationFooter } from '#/components/PaginationFooter'
import { RoutePending } from '#/components/RoutePending'
import { SearchInput } from '#/components/SearchInput'

export const Route = createFileRoute('/settings/my-organizations')({
  component: SettingsMyOrganizationsPage,
  pendingComponent: RoutePending,
})

function SettingsMyOrganizationsPage() {
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

    toast.error(organizations.error instanceof Error ? organizations.error.message : 'Failed to load organizations')
  }, [organizations.error])

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="space-y-2">
          <h2 className="text-2xl font-semibold tracking-tight">My Organizations</h2>
          <p className="text-sm text-muted-foreground">Choose an organization to continue.</p>
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
            <Card key={index}>
              <CardContent className="flex min-h-56 flex-col justify-between gap-6">
                <div className="flex items-start gap-4">
                  <div className="flex size-12 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-semibold text-muted-foreground">
                    --
                  </div>
                  <div className="flex flex-1 flex-col gap-2">
                    <div className="h-5 w-32 rounded bg-muted" />
                    <div className="h-4 w-24 rounded bg-muted" />
                  </div>
                </div>
                <div className="h-9 w-full rounded bg-muted" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}

      {organizations.isError ? (
        <Card>
          <CardContent>
            <EmptyState icon={Building04Icon} message="Failed to load organizations" description="Refresh the page and try again." />
          </CardContent>
        </Card>
      ) : null}

      {!organizations.isLoading && !organizations.isError && items.length === 0 ? (
        <Card>
          <CardContent>
            <EmptyState
              icon={Building04Icon}
              message={query.q ? 'No organizations matched your search.' : 'No organizations found'}
              description={query.q ? 'Try a different name or slug.' : 'You do not belong to any organizations yet.'}
            />
          </CardContent>
        </Card>
      ) : null}

      {!organizations.isLoading && !organizations.isError && items.length > 0 ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {items.map((organization) => (
                <Card
                  key={organization.id}
                  className="h-full gap-4 py-4 transition-colors hover:border-foreground/20 hover:bg-muted/30"
                >
                  <CardHeader className="flex flex-row items-center gap-4 space-y-0">
                    <div className="flex size-12 shrink-0 items-center justify-center rounded-lg bg-muted text-sm font-semibold text-foreground">
                      {getInitials(organization.name, 'O')}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <CardTitle className="truncate text-base">{organization.name}</CardTitle>
                        </div>
                      </div>
                    </div>
                    <CardAction>
                      <Badge variant="outline" className="shrink-0 capitalize">
                        {organization.role}
                      </Badge>
                    </CardAction>
                  </CardHeader>
                <CardContent>
                  <div className="flex flex-col justify-center gap-4">
                    <span className="border w-full" />
                    <div className="flex h-16 items-center justify-around gap-4">
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted-foreground uppercase">Members</span>
                        <span className="text-lg font-semibold text-foreground">{organization.member_count}</span>
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted-foreground uppercase">Teams</span>
                        <span className="text-lg font-semibold text-foreground">{organization.team_count}</span>
                      </div>
                    </div>
                    <span className="border w-full" />
                  </div>
                </CardContent>
                <CardFooter className="flex justify-between">
                  <CardDescription className="truncate">@{organization.slug}</CardDescription>
                  <Button
                    className="w-auto px-4"
                    nativeButton={false}
                    render={<Link to="/orgs/$org_slug/workspaces" params={{ org_slug: organization.slug }} />}
                  >
                    Visit
                  </Button>
                </CardFooter>
              </Card>
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
