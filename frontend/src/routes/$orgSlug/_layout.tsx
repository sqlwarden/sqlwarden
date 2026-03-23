import { createFileRoute, Outlet } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useAuth } from '#/contexts/AuthContext'
import { OrgLayout } from '#/components/layouts/OrgLayout'
import { orgsApi } from '#/lib/api/orgs'

export const Route = createFileRoute('/$orgSlug/_layout')({
  component: OrgGuard,
})

function OrgGuard() {
  const { orgSlug } = Route.useParams()
  const { isAuthenticated, isLoading } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (isLoading) return
    if (!isAuthenticated) {
      // Check org SSO before redirecting
      orgsApi.getOrgAuthInfo(orgSlug).then(info => {
        if (info.has_sso) {
          // Future: show org SSO login. For now redirect to instance login with next param.
          navigate({ to: '/login', search: { next: `/${orgSlug}` } })
        } else {
          navigate({ to: '/login', search: { next: `/${orgSlug}` } })
        }
      }).catch(() => navigate({ to: '/login', search: { next: `/${orgSlug}` } }))
    }
  }, [isLoading, isAuthenticated, orgSlug])

  if (isLoading || !isAuthenticated) {
    return <div className="min-h-screen bg-zinc-950" />
  }

  return (
    <OrgLayout>
      <Outlet />
    </OrgLayout>
  )
}
