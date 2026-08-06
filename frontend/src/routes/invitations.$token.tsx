import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import type {
  AcceptOrganizationInvitationResponse,
  OrganizationInvitationDetails,
} from '#/lib/api/types'
import { getAccessToken, setAccessToken } from '#/lib/auth/access-token'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { useSession } from '#/hooks/use-session'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'

export const Route = createFileRoute('/invitations/$token')({
  component: InvitationPage,
})

function InvitationPage() {
  const { token } = Route.useParams()
  const queryClient = useQueryClient()
  const [values, setValues] = useState({ name: '', password: '' })
  const session = useSession(Boolean(getAccessToken()))
  const invitation = useQuery({
    queryKey: queryKeys.invitation(token),
    queryFn: () => api.get<OrganizationInvitationDetails>(`/api/v1/invitations/${token}`),
    retry: false,
  })
  const accept = useMutation({
    mutationFn: () =>
      api.post<AcceptOrganizationInvitationResponse>(`/api/v1/invitations/${token}/accept`, {
        name: values.name.trim(),
        password: values.password,
      }),
    onSuccess: (payload) => {
      if (payload.access_token) {
        clearAuthScopedQueryCache(queryClient)
        setAccessToken(payload.access_token)
      }
      window.location.replace(`/orgs/${payload.organization.slug}`)
    },
  })

  if (invitation.isLoading || session.isLoading) {
    return <InvitationShell title="Loading invitation…" />
  }
  if (invitation.isError || !invitation.data) {
    return (
      <InvitationShell
        title="Invitation unavailable"
        description={errorMessage(
          invitation.error,
          'This invitation is invalid or no longer available.',
        )}
      />
    )
  }

  const details = invitation.data
  if (details.status === 'expired') {
    return (
      <InvitationShell
        title="Invitation expired"
        description="Ask an organization administrator to resend the invitation."
      />
    )
  }
  const redirect = `/invitations/${token}`
  const fieldErrors = isApiError(accept.error) ? (accept.error.fieldErrors ?? {}) : {}

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-12">
      <div className="flex w-full max-w-md flex-col gap-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <Badge variant="outline">SQLWarden</Badge>
          <div className="flex flex-col gap-2">
            <h1 className="font-heading text-2xl font-semibold tracking-tight">
              Join {details.organization.name}
            </h1>
            <p className="text-sm text-muted-foreground">This invitation is for {details.email}.</p>
          </div>
        </div>
        <Card className="py-0">
          <CardHeader className="px-6 pt-6">
            <CardTitle>Accept invitation</CardTitle>
            <CardDescription>
              Membership provides baseline organization access. Workspace access is assigned
              separately.
            </CardDescription>
          </CardHeader>
          <CardContent className="px-6 pb-6">
            {details.account_exists && !session.data ? (
              <div className="flex flex-col gap-4">
                <p className="text-sm text-muted-foreground">
                  Sign in as {details.email}, then return here to accept.
                </p>
                <Button
                  nativeButton={false}
                  render={<a href={`/login?redirect=${encodeURIComponent(redirect)}`} />}
                >
                  Sign in to accept
                </Button>
              </div>
            ) : null}
            {details.account_exists && session.data && !details.authenticated_as_invitee ? (
              <p className="text-sm text-destructive">
                You are signed in with a different account. Sign out and use {details.email}.
              </p>
            ) : null}
            {details.account_exists && details.authenticated_as_invitee ? (
              <Button
                className="w-full"
                disabled={accept.isPending}
                onClick={() => accept.mutate()}
              >
                {accept.isPending ? 'Accepting…' : 'Accept invitation'}
              </Button>
            ) : null}
            {!details.account_exists ? (
              <form
                className="flex flex-col gap-5"
                onSubmit={(event) => {
                  event.preventDefault()
                  accept.mutate()
                }}
              >
                <div className="flex flex-col gap-2">
                  <label htmlFor="invite-name" className="text-sm font-medium">
                    Name
                  </label>
                  <Input
                    id="invite-name"
                    autoComplete="name"
                    aria-invalid={Boolean(fieldErrors.name)}
                    value={values.name}
                    onChange={(event) =>
                      setValues((current) => ({ ...current, name: event.target.value }))
                    }
                  />
                  {fieldErrors.name ? (
                    <p className="text-xs text-destructive">{fieldErrors.name}</p>
                  ) : null}
                </div>
                <div className="flex flex-col gap-2">
                  <label htmlFor="invite-password" className="text-sm font-medium">
                    Password
                  </label>
                  <Input
                    id="invite-password"
                    type="password"
                    autoComplete="new-password"
                    aria-invalid={Boolean(fieldErrors.password)}
                    value={values.password}
                    onChange={(event) =>
                      setValues((current) => ({ ...current, password: event.target.value }))
                    }
                  />
                  {fieldErrors.password ? (
                    <p className="text-xs text-destructive">{fieldErrors.password}</p>
                  ) : null}
                </div>
                {accept.error && Object.keys(fieldErrors).length === 0 ? (
                  <p className="text-sm text-destructive">
                    {errorMessage(accept.error, 'Failed to accept invitation')}
                  </p>
                ) : null}
                <Button
                  type="submit"
                  disabled={accept.isPending || !values.name.trim() || values.password.length < 8}
                >
                  {accept.isPending ? 'Creating account…' : 'Create account and join'}
                </Button>
              </form>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </main>
  )
}

function InvitationShell({ title, description }: { title: string; description?: string }) {
  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          {description ? <CardDescription>{description}</CardDescription> : null}
        </CardHeader>
      </Card>
    </main>
  )
}
