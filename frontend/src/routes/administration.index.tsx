import { Link, createFileRoute } from '@tanstack/react-router'
import { Icon, type AppIcon } from '#/lib/icons'

export const Route = createFileRoute('/administration/')({
  component: AdministrationIndexPage,
})

type AdminOverviewCard = {
  to:
    | '/administration/users'
    | '/administration/administrators'
    | '/administration/organizations'
    | '/administration/instance'
  label: string
  description: string
  icon: AppIcon
}

const cards: AdminOverviewCard[] = [
  {
    to: '/administration/users',
    label: 'Users',
    description: 'Every account on this instance.',
    icon: 'user-multiple-02',
  },
  {
    to: '/administration/administrators',
    label: 'Administrators',
    description: 'Accounts with instance-admin access.',
    icon: 'shield-user',
  },
  {
    to: '/administration/organizations',
    label: 'Organizations',
    description: 'All organizations hosted on this instance.',
    icon: 'building-04',
  },
  {
    to: '/administration/instance',
    label: 'Settings',
    description: 'Instance-wide configuration.',
    icon: 'settings-02',
  },
]

function AdministrationIndexPage() {
  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Administration</h1>
        <p className="text-sm text-muted-foreground">Instance-wide users, orgs, and settings.</p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {cards.map((card) => (
          <Link
            key={card.to}
            to={card.to}
            className="group flex flex-col rounded-lg border border-border bg-card text-card-foreground transition-all hover:border-foreground/20 hover:bg-muted/20 hover:shadow-sm"
          >
            <div className="flex flex-1 items-start gap-3 p-5">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                <Icon name={card.icon} size={20} />
              </div>
              <div className="min-w-0 flex-1 pt-0.5">
                <p className="truncate font-semibold leading-tight tracking-tight transition-colors group-hover:text-primary">
                  {card.label}
                </p>
                <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
                  {card.description}
                </p>
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
