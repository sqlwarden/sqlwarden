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

      <div className="flex flex-col divide-y divide-border rounded-lg border border-border bg-card">
        {cards.map((card) => (
          <Link
            key={card.to}
            to={card.to}
            className="group flex items-center gap-3 p-4 text-card-foreground transition-colors first:rounded-t-lg last:rounded-b-lg hover:bg-muted/40"
          >
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <Icon name={card.icon} size={18} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate font-heading font-medium leading-tight tracking-tight transition-colors group-hover:text-primary">
                {card.label}
              </p>
              <p className="mt-0.5 truncate text-xs leading-relaxed text-muted-foreground">
                {card.description}
              </p>
            </div>
            <Icon
              name="arrow-right-01"
              size={16}
              className="shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
            />
          </Link>
        ))}
      </div>
    </div>
  )
}
