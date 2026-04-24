import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, Navigate, Outlet, createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import { Briefcase01Icon, Building04Icon, Key01Icon, Logout03Icon, PaintBoardIcon, Settings02Icon, ShieldUserIcon, User02Icon } from '@hugeicons/core-free-icons'
import { useSession } from '#/hooks/use-session'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { api } from '#/lib/api/client'
import { queryKeys } from '#/lib/api/query'
import type { SessionResponse } from '#/lib/api/types'
import { clearAccessToken, getAccessToken } from '#/lib/auth/access-token'
import { useTheme } from '#/components/theme-provider'
import { Avatar, AvatarFallback } from '#/components/ui/avatar'
import { Button } from '#/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from '#/components/ui/sidebar'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'

export const Route = createFileRoute('/settings')({
  component: SettingsLayout,
})

type NavItem = { to: string; label: string; icon: typeof User02Icon }
type Theme = 'dark' | 'light' | 'system'
type ContentLayout = 'centered' | 'full-width'
type NavbarStyle = 'sticky' | 'scroll'
type SidebarStyle = 'sidebar' | 'inset' | 'floating'

type SettingsPreferences = {
  themeMode: Theme
  contentLayout: ContentLayout
  navbarStyle: NavbarStyle
  sidebarStyle: SidebarStyle
}

const preferenceKeys = {
  themeMode: 'sqlwarden.preference.theme_mode',
  contentLayout: 'sqlwarden.preference.content_layout',
  navbarStyle: 'sqlwarden.preference.navbar_style',
  sidebarStyle: 'sqlwarden.preference.sidebar_style',
} as const

const defaultPreferences: SettingsPreferences = {
  themeMode: 'system',
  contentLayout: 'centered',
  navbarStyle: 'scroll',
  sidebarStyle: 'sidebar',
}

const accountItems: NavItem[] = [
  { to: '/settings/account', label: 'Account', icon: User02Icon },
  { to: '/settings/my-organizations', label: 'My Organizations', icon: Briefcase01Icon },
  { to: '/settings/api-tokens', label: 'API Tokens', icon: Key01Icon },
]

const adminItems: NavItem[] = [
  { to: '/settings/administrators', label: 'Administrators', icon: ShieldUserIcon },
  { to: '/settings/organizations', label: 'Organizations', icon: Building04Icon },
]

function SettingsLayout() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const { setTheme } = useTheme()
  const [preferences, setPreferences] = useState<SettingsPreferences>(() => readPreferences())
  const [initialOpen] = useState(() => {
    const cookie = document.cookie.split('; ').find((row) => row.startsWith('sidebar_state='))
    return cookie ? cookie.split('=')[1] === 'true' : true
  })

  useEffect(() => {
    applyPreferences(preferences)
    setTheme(preferences.themeMode)
  }, [preferences, setTheme])

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading...</div>
      </main>
    )
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (!hasToken || !session.data) {
    return <Navigate to="/login" replace />
  }

  return (
    <SidebarProvider
      defaultOpen={initialOpen}
      style={{
        '--sidebar-width': '15rem',
        '--sidebar-width-icon': '3rem',
      } as React.CSSProperties}
    >
      <SettingsSidebar
        session={session.data}
        pathname={pathname}
        preferences={preferences}
        setPreferences={setPreferences}
      />
      <SidebarInset className="min-w-0 bg-background">
        <main className={preferences.contentLayout === 'centered' ? 'mx-auto min-h-svh w-full max-w-screen-2xl px-4 py-6 md:px-6' : 'min-h-svh px-4 py-6 md:px-6'}>
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

function SettingsSidebar({
  session,
  pathname,
  preferences,
  setPreferences,
}: {
  session: SessionResponse
  pathname: string
  preferences: SettingsPreferences
  setPreferences: React.Dispatch<React.SetStateAction<SettingsPreferences>>
}) {
  return (
    <Sidebar collapsible="icon" variant={preferences.sidebarStyle}>
      <SidebarHeader>
        <div className="flex items-center gap-2">
          <SidebarMenu className="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
            <SidebarMenuItem>
              <SidebarMenuButton tooltip="Settings">
                <HugeiconsIcon icon={Settings02Icon} strokeWidth={2} />
                <span className="font-semibold">Settings</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
          <SidebarTrigger
            className="shrink-0 cursor-pointer"
            aria-label="Toggle sidebar"
          />
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SettingsNavSection label="Account" items={accountItems} pathname={pathname} />
        {session.is_instance_admin ? (
          <SettingsNavSection label="Instance Admin" items={adminItems} pathname={pathname} />
        ) : null}
      </SidebarContent>

      <SidebarFooter>
        <SettingsPreferencesPopover
          preferences={preferences}
          setPreferences={setPreferences}
        />
        <SettingsNavUser session={session} />
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  )
}

function SettingsNavSection({
  label,
  items,
  pathname,
}: {
  label: string
  items: NavItem[]
  pathname: string
}) {
  return (
    <div className="flex flex-col gap-1 px-2 py-1 group-data-[collapsible=icon]:px-2">
      <div className="flex h-8 items-center gap-2 px-2 text-xs font-medium tracking-wide text-sidebar-foreground/70 uppercase group-data-[collapsible=icon]:hidden">
        <span>{label}</span>
        <span className="h-px flex-1 bg-sidebar-border" />
      </div>
      <SidebarMenu>
        {items.map((item) => (
          <SettingsNavItem key={item.to} item={item} isActive={pathname === item.to} />
        ))}
      </SidebarMenu>
    </div>
  )
}

function SettingsNavItem({
  item,
  isActive,
}: {
  item: NavItem
  isActive: boolean
}) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} />}
        isActive={isActive}
        tooltip={item.label}
      >
        <HugeiconsIcon icon={item.icon} strokeWidth={2} />
        <span>{item.label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function SettingsNavUser({ session }: { session: SessionResponse }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const initials = accountInitials(session.account.name)

  const logout = useMutation({
    mutationFn: async () => api.post<void>('/api/v1/auth/logout'),
    onSettled: async () => {
      clearAccessToken()
      await queryClient.invalidateQueries({ queryKey: queryKeys.session() })
      await navigate({ to: '/login', replace: true })
    },
  })

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton
                size="lg"
                className="data-popup-open:bg-sidebar-accent data-popup-open:text-sidebar-accent-foreground"
              />
            }
          >
            <Avatar className="rounded-lg">
              <AvatarFallback className="rounded-lg">{initials}</AvatarFallback>
            </Avatar>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{session.account.name}</span>
              <span className="truncate text-xs text-muted-foreground">{session.account.email}</span>
            </div>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--anchor-width) min-w-60 rounded-lg"
            side="right"
            align="end"
            sideOffset={4}
          >
            <DropdownMenuGroup>
              <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-2 py-1.5 text-left text-sm">
                  <Avatar className="rounded-lg">
                    <AvatarFallback className="rounded-lg">{initials}</AvatarFallback>
                  </Avatar>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium text-foreground">{session.account.name}</span>
                    <span className="truncate text-xs text-muted-foreground">{session.account.email}</span>
                  </div>
                </div>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem render={<Link to="/account" />}>
                <HugeiconsIcon icon={User02Icon} strokeWidth={2} />
                Profile
              </DropdownMenuItem>
              {session.is_instance_admin ? (
                <DropdownMenuItem render={<Link to="/settings/administrators" />}>
                  <HugeiconsIcon icon={ShieldUserIcon} strokeWidth={2} />
                  Administration
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              disabled={logout.isPending}
              onClick={() => {
                void logout.mutateAsync()
              }}
            >
              <HugeiconsIcon icon={Logout03Icon} strokeWidth={2} />
              {logout.isPending ? 'Signing out...' : 'Sign out'}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}

function SettingsPreferencesPopover({
  preferences,
  setPreferences,
}: {
  preferences: SettingsPreferences
  setPreferences: React.Dispatch<React.SetStateAction<SettingsPreferences>>
}) {
  function updatePreference<Key extends keyof SettingsPreferences>(key: Key, value: SettingsPreferences[Key]) {
    window.localStorage.setItem(preferenceKeys[key], value)
    setPreferences((current) => ({
      ...current,
      [key]: value,
    }))
  }

  function restoreDefaults() {
    Object.entries(preferenceKeys).forEach(([key, storageKey]) => {
      const typedKey = key as keyof SettingsPreferences
      window.localStorage.setItem(storageKey, defaultPreferences[typedKey])
    })
    setPreferences(defaultPreferences)
  }

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            className="w-full justify-start gap-2 group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          />
        }
      >
        <HugeiconsIcon icon={PaintBoardIcon} strokeWidth={2} />
        <span className="group-data-[collapsible=icon]:hidden">UI Preferences</span>
      </PopoverTrigger>
      <PopoverContent side="right" align="end" className="w-80">
        <div className="flex flex-col gap-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex flex-col gap-1.5">
              <h4 className="text-sm font-medium leading-none">Preferences</h4>
              <p className="text-xs text-muted-foreground">Temporary controls for layout experiments.</p>
            </div>
            <Button type="button" variant="ghost" size="sm" onClick={restoreDefaults}>
              Reset
            </Button>
          </div>

          <div className="flex flex-col gap-3 **:data-[slot=toggle-group]:w-full **:data-[slot=toggle-group-item]:flex-1 **:data-[slot=toggle-group-item]:text-xs">
            <PreferenceToggle
              label="Theme Mode"
              value={preferences.themeMode}
              options={['light', 'dark', 'system']}
              onValueChange={(value) => updatePreference('themeMode', value as Theme)}
            />

            <PreferenceToggle
              label="Page Layout"
              value={preferences.contentLayout}
              options={['centered', 'full-width']}
              labels={{ centered: 'Centered', 'full-width': 'Full Width' }}
              onValueChange={(value) => updatePreference('contentLayout', value as ContentLayout)}
            />

            <PreferenceToggle
              label="Navbar Behavior"
              value={preferences.navbarStyle}
              options={['sticky', 'scroll']}
              labels={{ sticky: 'Sticky', scroll: 'Scroll' }}
              onValueChange={(value) => updatePreference('navbarStyle', value as NavbarStyle)}
            />

            <PreferenceToggle
              label="Sidebar Style"
              value={preferences.sidebarStyle}
              options={['inset', 'sidebar', 'floating']}
              labels={{ inset: 'Inset', sidebar: 'Sidebar', floating: 'Floating' }}
              onValueChange={(value) => updatePreference('sidebarStyle', value as SidebarStyle)}
            />
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function PreferenceToggle({
  label,
  value,
  options,
  labels,
  onValueChange,
}: {
  label: string
  value: string
  options: string[]
  labels?: Record<string, string>
  onValueChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">{label}</div>
      <ToggleGroup
        size="sm"
        variant="outline"
        value={[value]}
        onValueChange={(nextValue) => {
          const selected = nextValue[0]
          if (selected) onValueChange(selected)
        }}
      >
        {options.map((option) => (
          <ToggleGroupItem key={option} value={option} aria-label={labels?.[option] ?? option}>
            {labels?.[option] ?? titleCase(option)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  )
}

function readPreferences(): SettingsPreferences {
  return {
    themeMode: readPreference(preferenceKeys.themeMode, ['dark', 'light', 'system'], defaultPreferences.themeMode),
    contentLayout: readPreference(preferenceKeys.contentLayout, ['centered', 'full-width'], defaultPreferences.contentLayout),
    navbarStyle: readPreference(preferenceKeys.navbarStyle, ['sticky', 'scroll'], defaultPreferences.navbarStyle),
    sidebarStyle: readPreference(preferenceKeys.sidebarStyle, ['sidebar', 'inset', 'floating'], defaultPreferences.sidebarStyle),
  }
}

function readPreference<Value extends string>(key: string, allowed: Value[], fallback: Value) {
  const stored = window.localStorage.getItem(key)
  return stored && allowed.includes(stored as Value) ? stored as Value : fallback
}

function applyPreferences(preferences: SettingsPreferences) {
  const root = document.documentElement
  root.setAttribute('data-theme-mode', preferences.themeMode)
  root.removeAttribute('data-theme-preset')
  root.removeAttribute('data-font')
  root.setAttribute('data-content-layout', preferences.contentLayout)
  root.setAttribute('data-navbar-style', preferences.navbarStyle)
  root.setAttribute('data-sidebar-variant', preferences.sidebarStyle)
  root.setAttribute('data-sidebar-collapsible', 'icon')
}

function titleCase(value: string) {
  return value
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function accountInitials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)

  if (parts.length === 0) {
    return 'U'
  }

  return parts
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}
