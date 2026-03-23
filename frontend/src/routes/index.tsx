import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useAuth } from '#/contexts/AuthContext'
import { useUserOrgs } from '#/lib/queries/useAuth'

export const Route = createFileRoute('/')({ component: RootRedirect })

function RootRedirect() {
  const { isAuthenticated, isLoading } = useAuth()
  const { data: orgs } = useUserOrgs()
  const navigate = useNavigate()

  useEffect(() => {
    if (isLoading) return
    if (!isAuthenticated) {
      navigate({ to: '/login' })
      return
    }
    // Redirect to first org if available
    if (orgs && orgs.length > 0) {
      navigate({ to: '/$orgSlug', params: { orgSlug: orgs[0].slug } })
    }
    // If no orgs yet, stay on loading screen
  }, [isLoading, isAuthenticated, orgs, navigate])

  return <div className="min-h-screen bg-zinc-950" />
}
