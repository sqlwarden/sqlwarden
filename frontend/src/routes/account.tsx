import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/account')({
  component: AccountPage,
})

function AccountPage() {
  return <div className="text-sm text-muted-foreground">Profile works!</div>
}
