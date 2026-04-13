import { Navigate, createFileRoute } from '@tanstack/react-router'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { getAccessToken } from '#/lib/auth/access-token'

export const Route = createFileRoute('/')({ component: App })

function App() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())

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

  if (hasToken) {
    return <Navigate to="/account" replace />
  }

  return <Navigate to="/login" replace />
}
