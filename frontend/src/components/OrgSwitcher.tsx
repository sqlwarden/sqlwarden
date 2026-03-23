import { useNavigate } from '@tanstack/react-router'
import { useAuth } from '#/contexts/AuthContext'
import { useUserOrgs } from '#/lib/queries/useAuth'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuGroup, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { ChevronDown } from 'lucide-react'
import type { Tenant } from '#/lib/types/org'

interface OrgSwitcherProps { currentOrg: Tenant }

export function OrgSwitcher({ currentOrg }: OrgSwitcherProps) {
  const { user } = useAuth()
  const { data: orgs = [] } = useUserOrgs()
  const navigate = useNavigate()

  // Personal org heuristic: the personal org slug is derived from the email username at registration.
  const emailUsername = user?.email.split('@')[0] ?? ''
  const personalOrg = orgs.find(o => o.slug === emailUsername)
  const teamOrgs = orgs.filter(o => o.slug !== emailUsername)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 w-full px-3 py-2 rounded-md bg-zinc-800 hover:bg-zinc-700 transition-colors">
        <div className="h-6 w-6 rounded bg-blue-600 flex items-center justify-center flex-shrink-0">
          <span className="text-xs font-bold text-white">{currentOrg.slug[0].toUpperCase()}</span>
        </div>
        <span className="flex-1 text-sm font-medium text-zinc-100 truncate text-left">{currentOrg.name}</span>
        <ChevronDown className="h-4 w-4 text-zinc-400 flex-shrink-0" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {personalOrg && (
          <>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-zinc-500 uppercase tracking-wider">Personal</DropdownMenuLabel>
              <DropdownMenuItem onClick={() => navigate({ to: '/$orgSlug', params: { orgSlug: personalOrg.slug } })}>
                <div className="h-4 w-4 rounded-full bg-green-600 flex items-center justify-center mr-2 flex-shrink-0">
                  <span className="text-[8px] font-bold text-white">{personalOrg.slug[0].toUpperCase()}</span>
                </div>
                {personalOrg.name}
                {currentOrg.slug === personalOrg.slug && <span className="ml-auto text-blue-400 text-xs">✓</span>}
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
          </>
        )}
        {teamOrgs.length > 0 && (
          <>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-zinc-500 uppercase tracking-wider">Organizations</DropdownMenuLabel>
              {teamOrgs.map(org => (
                <DropdownMenuItem key={org.id} onClick={() => navigate({ to: '/$orgSlug', params: { orgSlug: org.slug } })}>
                  <div className="h-4 w-4 rounded bg-blue-600 flex items-center justify-center mr-2 flex-shrink-0">
                    <span className="text-[8px] font-bold text-white">{org.slug[0].toUpperCase()}</span>
                  </div>
                  {org.name}
                  {currentOrg.slug === org.slug && <span className="ml-auto text-blue-400 text-xs">✓</span>}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
          </>
        )}
        {user?.is_superadmin && (
          <DropdownMenuItem disabled>
            + Create organization
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
