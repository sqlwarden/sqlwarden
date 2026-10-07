import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { editionRegistry } from '@edition'
import { EmptyState } from '#/components/EmptyState'
import { RoutePending } from '#/components/RoutePending'
import { NavigateToLogin } from '#/components/auth/NavigateToLogin'
import { EditionUpsell } from '#/edition/EditionUpsell'
import { resolveEditionPage } from '#/edition/resolve'
import { editionCapabilitiesQueryOptions } from '#/lib/api/query'
import { getAccessToken } from '#/lib/auth/access-token'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/ee/$')({
  component: EditionPage,
  pendingComponent: RoutePending,
})

function EditionPage() {
  usePageTitle('Enterprise')
  const { _splat: path = '' } = Route.useParams()
  const hasToken = Boolean(getAccessToken())
  const capabilities = useQuery({ ...editionCapabilitiesQueryOptions(), enabled: hasToken })

  if (!hasToken) return <NavigateToLogin />
  if (!capabilities.data) return <RoutePending />

  const page = resolveEditionPage(editionRegistry, capabilities.data, path)
  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-6 py-10">
      {page.kind === 'route' ? <page.component /> : null}
      {page.kind === 'locked' ? <EditionUpsell feature={page.feature} /> : null}
      {page.kind === 'not-found' ? <EmptyState message="Page not found" /> : null}
    </main>
  )
}
