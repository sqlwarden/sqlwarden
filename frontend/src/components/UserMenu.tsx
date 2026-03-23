import { useNavigate } from '@tanstack/react-router'
import { useAuth } from '#/contexts/AuthContext'
import { Avatar, AvatarFallback } from '#/components/ui/avatar'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuSeparator, DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'

export function UserMenu() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const initials = user?.name
    .split(' ')
    .map(n => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2) ?? '?'

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 w-full px-2 py-2 rounded-md hover:bg-zinc-800 transition-colors text-left">
        <Avatar className="h-7 w-7">
          <AvatarFallback className="text-xs bg-zinc-700">{initials}</AvatarFallback>
        </Avatar>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-zinc-100 truncate">{user?.name}</p>
          <p className="text-xs text-zinc-400 truncate">{user?.email}</p>
        </div>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuItem disabled>Profile (coming soon)</DropdownMenuItem>
        <DropdownMenuSeparator />
        {user?.is_superadmin && (
          <>
            <DropdownMenuItem onClick={() => navigate({ to: '/admin' })}>Go to Admin</DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        <DropdownMenuItem
          className="text-red-400 focus:text-red-400"
          onClick={async () => { await logout(); navigate({ to: '/login' }) }}
        >
          Logout
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
