import { createFileRoute, Outlet } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useAuth } from '#/contexts/AuthContext'
import { AdminLayout } from '#/components/layouts/AdminLayout'

export const Route = createFileRoute('/admin/_layout')({
  component: AdminGuard,
})

function AdminGuard() {
  const { isAuthenticated, isLoading, user } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (isLoading) return
    if (!isAuthenticated) { navigate({ to: '/login' }); return }
    if (!user?.is_superadmin) { navigate({ to: '/' }); return }
  }, [isLoading, isAuthenticated, user])

  if (isLoading || !isAuthenticated || !user?.is_superadmin) {
    return <div className="min-h-screen bg-zinc-950" />
  }

  return (
    <AdminLayout>
      <Outlet />
    </AdminLayout>
  )
}
