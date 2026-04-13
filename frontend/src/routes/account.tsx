import { Navigate, createFileRoute } from '@tanstack/react-router'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { getAccessToken } from '#/lib/auth/access-token'
import { Badge } from '#/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'

export const Route = createFileRoute('/account')({
  component: AccountPage,
})

function AccountPage() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)

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

  if (session.isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading account…</div>
      </main>
    )
  }

  if (!session.data) {
    return <Navigate to="/login" replace />
  }

  const { account, is_instance_admin: isInstanceAdmin, organizations, personal_spaces_enabled: personalSpacesEnabled } = session.data

  return (
    <main className="container mx-auto max-w-3xl px-4 py-12">
      <div className="space-y-6">
        <div className="space-y-3">
          <Badge variant="outline">Current user</Badge>
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight">{account.name}</h1>
            <p className="text-sm text-muted-foreground">{account.email}</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Session details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="flex items-center justify-between gap-4 border-b pb-3">
              <span className="text-muted-foreground">Account ID</span>
              <span>{account.id}</span>
            </div>
            <div className="flex items-center justify-between gap-4 border-b pb-3">
              <span className="text-muted-foreground">Instance admin</span>
              <span>{isInstanceAdmin ? 'Yes' : 'No'}</span>
            </div>
            <div className="flex items-center justify-between gap-4 border-b pb-3">
              <span className="text-muted-foreground">Organizations</span>
              <span>{organizations.length}</span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">Personal spaces enabled</span>
              <span>{personalSpacesEnabled ? 'Yes' : 'No'}</span>
            </div>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
