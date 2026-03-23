import { Link, useRouterState } from '@tanstack/react-router'
import { UserMenu } from '#/components/UserMenu'
import { cn } from '#/lib/utils'
import { Building2, Users, ShieldCheck, Settings, LayoutDashboard } from 'lucide-react'

const NAV = [
  { to: '/admin', label: 'Dashboard', icon: LayoutDashboard, exact: true },
  { to: '/admin/organizations', label: 'Organizations', icon: Building2 },
  { to: '/admin/accounts', label: 'Accounts', icon: Users },
  { to: '/admin/auth-settings', label: 'Auth Settings', icon: ShieldCheck },
  { to: '/admin/instance-settings', label: 'Instance Settings', icon: Settings },
]

export function AdminLayout({ children }: { children: React.ReactNode }) {
  const { location: { pathname } } = useRouterState()
  return (
    <div className="flex h-screen bg-zinc-950 text-zinc-100">
      <aside className="w-[220px] flex-shrink-0 border-r border-zinc-800 flex flex-col">
        <div className="px-4 py-4 border-b border-zinc-800">
          <div className="flex items-center gap-2">
            <div className="h-6 w-6 rounded bg-zinc-100 flex items-center justify-center">
              <span className="text-[10px] font-bold text-zinc-900">SW</span>
            </div>
            <div>
              <p className="text-sm font-semibold">SQLWarden</p>
              <p className="text-xs text-orange-400">Admin Panel</p>
            </div>
          </div>
        </div>
        <nav className="flex-1 px-2 py-3 space-y-1 overflow-y-auto">
          {NAV.map(item => {
            const active = item.exact ? pathname === item.to : pathname.startsWith(item.to)
            return (
              <Link key={item.to} to={item.to as any} className={cn(
                'flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors',
                active ? 'bg-orange-500/10 text-orange-400' : 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800'
              )}>
                <item.icon className="h-4 w-4 flex-shrink-0" />
                {item.label}
              </Link>
            )
          })}
        </nav>
        <div className="px-2 py-3 border-t border-zinc-800 space-y-1">
          <Link to="/" className="flex items-center gap-2 px-3 py-2 text-xs text-zinc-500 hover:text-zinc-300 transition-colors">
            ← Back to app
          </Link>
          <UserMenu />
        </div>
      </aside>
      <main className="flex-1 overflow-y-auto">{children}</main>
    </div>
  )
}
