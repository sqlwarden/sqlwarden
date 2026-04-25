import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/orgs/$org_slug/')({
  component: OrganizationHomePage,
})

function OrganizationHomePage() {
  return <PlaceholderPage title="Home" />
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="flex flex-col gap-2">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="text-sm text-muted-foreground">{title} works!</p>
    </div>
  )
}

