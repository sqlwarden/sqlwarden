import { createFileRoute, Outlet } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useAuth } from '#/contexts/AuthContext'
import { OrgLayout } from '#/components/layouts/OrgLayout'

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
      navigate({ to: '/login', search: { next: `/${orgSlug}` } })
    }
  }, [isLoading, isAuthenticated, orgSlug, navigate])

  if (isLoading || !isAuthenticated) {
    return <div className="min-h-screen bg-zinc-950" />
  }

  return (
    <OrgLayout>
      <Outlet />
    </OrgLayout>
  )
}
