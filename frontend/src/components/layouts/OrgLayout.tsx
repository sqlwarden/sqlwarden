import { Link, useParams, useRouterState } from '@tanstack/react-router'
import { UserMenu } from '#/components/UserMenu'
import { OrgSwitcher } from '#/components/OrgSwitcher'
import { useOrg } from '#/lib/queries/useOrg'
import { cn } from '#/lib/utils'
import { LayoutDashboard, Users, GitBranch, Shield, Settings } from 'lucide-react'

export function OrgLayout({ children }: { children: React.ReactNode }) {
  const { orgSlug } = useParams({ strict: false }) as { orgSlug: string }
  const { data: org } = useOrg(orgSlug)
  const { location: { pathname } } = useRouterState()

  const NAV = [
    { to: `/${orgSlug}`, label: 'Overview', icon: LayoutDashboard, exact: true },
    { to: `/${orgSlug}/members`, label: 'Members', icon: Users },
    { to: `/${orgSlug}/teams`, label: 'Teams', icon: GitBranch },
    { to: `/${orgSlug}/roles`, label: 'Roles', icon: Shield },
    { separator: true },
    { to: `/${orgSlug}/settings`, label: 'Settings', icon: Settings },
  ] as const

  return (
    <div className="flex h-screen bg-zinc-950 text-zinc-100">
      <aside className="w-[220px] flex-shrink-0 border-r border-zinc-800 flex flex-col">
        <div className="px-2 py-3 border-b border-zinc-800">
          {org
            ? <OrgSwitcher currentOrg={org} />
            : <div className="h-10 rounded-md bg-zinc-800 animate-pulse" />
          }
        </div>
        <nav className="flex-1 px-2 py-3 space-y-1 overflow-y-auto">
          {NAV.map((item, i) => {
            if ('separator' in item) return <div key={i} className="my-1 border-t border-zinc-800" />
            const active = item.exact ? pathname === item.to : pathname.startsWith(item.to)
            return (
              <Link key={item.to} to={item.to as any} className={cn(
                'flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors',
                active ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800'
              )}>
                <item.icon className="h-4 w-4 flex-shrink-0" />
                {item.label}
              </Link>
            )
          })}
        </nav>
        <div className="px-2 py-3 border-t border-zinc-800">
          <UserMenu />
        </div>
      </aside>
      <main className="flex-1 overflow-y-auto">{children}</main>
    </div>
  )
}
