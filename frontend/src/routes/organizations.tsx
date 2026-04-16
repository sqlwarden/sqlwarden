import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Navigate, createFileRoute } from '@tanstack/react-router'
import { Building2, Search, X } from 'lucide-react'
import { toast } from 'sonner'
import { useLayoutWidth } from '#/components/layout-width-provider'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { accountOrganizationsQueryOptions } from '#/lib/api/query'
import { getAccessToken } from '#/lib/auth/access-token'
import type { ListQuery } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { Button } from '#/components/ui/button'
import { Badge } from '#/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle, CardAction, CardFooter } from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Separator } from '#/components/ui/separator'

export const Route = createFileRoute('/organizations')({
  component: OrganizationsPage,
})

function OrganizationsPage() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const { isExpanded } = useLayoutWidth()
  const [searchText, setSearchText] = useState('')
  const [query, setQuery] = useState<ListQuery>({
    page: 1,
    page_size: 12,
    sort: 'name',
    order: 'asc',
    q: '',
  })

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQuery((current) => {
        const nextSearch = searchText.trim()
        if ((current.q ?? '') === nextSearch) {
          return current
        }

        return {
          ...current,
          page: 1,
          q: nextSearch,
        }
      })
    }, 300)

    return () => window.clearTimeout(timer)
  }, [searchText])

  const organizations = useQuery({
    ...accountOrganizationsQueryOptions(query),
    enabled: Boolean(hasToken && setupStatus.data?.configured),
  })

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

  if (setupStatus.isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading…</div>
      </main>
    )
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (!hasToken) {
    return <Navigate to="/login" replace />
  }

  function clearSearch() {
    setSearchText('')
  }

  return (
    <main
      className={cn(
        'py-12',
        isExpanded ? 'w-full px-4 sm:px-6' : 'container mx-auto max-w-6xl px-4',
      )}
    >
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-8">
        <div className="flex flex-col gap-3">
          <div className="space-y-2">
            <h1 className="text-3xl font-semibold tracking-tight">Organizations</h1>
            <p className="text-sm text-muted-foreground">Choose an organization to continue.</p>
          </div>

          <div className="relative max-w-md">
            <Search className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={searchText}
              onChange={(event) => setSearchText(event.target.value)}
              placeholder="Search organizations"
              className="pe-9 ps-9"
            />
            {searchText ? (
              <button
                type="button"
                aria-label="Clear search"
                className="absolute end-3 top-1/2 inline-flex size-4 -translate-y-1/2 cursor-pointer items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
                onClick={clearSearch}
              >
                <X className="size-4" />
              </button>
            ) : null}
          </div>
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
            <CardContent className="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
              <Building2 className="size-10 text-muted-foreground" />
              <div className="space-y-1">
                <p className="font-medium text-foreground">Failed to load organizations</p>
                <p className="text-sm text-muted-foreground">Refresh the page and try again.</p>
              </div>
            </CardContent>
          </Card>
        ) : null}

        {!organizations.isLoading && !organizations.isError && items.length === 0 ? (
          <Card>
            <CardContent className="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
              <Building2 className="size-10 text-muted-foreground" />
              <div className="space-y-1">
                <p className="font-medium text-foreground">
                  {query.q ? 'No organizations matched your search.' : 'No organizations found'}
                </p>
                <p className="text-sm text-muted-foreground">
                  {query.q ? 'Try a different name or slug.' : 'You do not belong to any organizations yet.'}
                </p>
              </div>
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
                      {organizationInitials(organization.name)}
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
                  <CardContent className="">
                    <div className="flex flex-col justify-center gap-4">
                      <span className="border w-full"></span>
                      <div className="flex justify-around items-center gap-4 h-16">
                        <div className="flex flex-col gap-1">
                          <span className="text-xs text-muted-foreground uppercase">Members</span>
                          <span className="text-lg font-semibold text-foreground">{organization.member_count}</span>
                        </div>
                        <div className="flex flex-col gap-1">
                          <span className="text-xs text-muted-foreground uppercase">Teams</span>
                          <span className="text-lg font-semibold text-foreground">{organization.team_count}</span>
                        </div>
                      </div>
                      <span className="border w-full"></span>
                    </div>
                  </CardContent>
                  <CardFooter className="flex justify-between">
                      <CardDescription className="truncate">@{organization.slug}</CardDescription>
                      <Button
                        className="w-auto px-4"
                        render={<Link to="/orgs/$org_slug/workspaces" params={{ org_slug: organization.slug }} />}
                      >
                        Visit
                      </Button>
                  </CardFooter>
                </Card>
              ))}
            </div>

            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-muted-foreground">
                {total === 0 ? '0 organizations' : `${(page - 1) * pageSize + 1}-${Math.min(page * pageSize, total)} of ${total} organizations`}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  onClick={() => setQuery((current) => ({ ...current, page: Math.max(1, Number(current.page ?? 1) - 1) }))}
                  disabled={page <= 1 || organizations.isFetching}
                >
                  Previous
                </Button>
                <div className="min-w-20 text-center text-sm text-muted-foreground">
                  Page {page} of {pageCount}
                </div>
                <Button
                  variant="outline"
                  onClick={() => setQuery((current) => ({ ...current, page: Number(current.page ?? 1) + 1 }))}
                  disabled={page >= pageCount || organizations.isFetching}
                >
                  Next
                </Button>
              </div>
            </div>
          </>
        ) : null}
      </div>
    </main>
  )
}

function organizationInitials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return 'O'
  }

  return parts
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}
