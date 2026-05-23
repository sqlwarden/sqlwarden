import { useEffect, useState } from 'react'
import type { Dispatch, ReactNode, SetStateAction } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  Logout03Icon,
  PaintBoardIcon,
  ShieldUserIcon,
  User02Icon,
} from '@hugeicons/core-free-icons'
import type { SessionResponse } from '#/lib/api/types'
import { api } from '#/lib/api/client'
import { clearAccessToken } from '#/lib/auth/access-token'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { InitialsAvatar } from '#/components/InitialsAvatar'
import { useTheme } from '#/components/theme-provider'
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
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarTrigger,
} from '#/components/ui/sidebar'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'

export type AppShellTheme = 'dark' | 'light' | 'system'
export type AppShellContentLayout = 'centered' | 'full-width'
export type AppShellNavbarStyle = 'sticky' | 'scroll'
export type AppShellSidebarStyle = 'sidebar' | 'inset' | 'floating'

export type AppShellPreferences = {
  themeMode: AppShellTheme
  contentLayout: AppShellContentLayout
  navbarStyle: AppShellNavbarStyle
  sidebarStyle: AppShellSidebarStyle
}

export type AppShellNavItem = {
  to: string
  label: string
  icon: typeof User02Icon
  params?: Record<string, string>
}

const preferenceKeys = {
  themeMode: 'sqlwarden.preference.theme_mode',
  contentLayout: 'sqlwarden.preference.content_layout',
  navbarStyle: 'sqlwarden.preference.navbar_style',
  sidebarStyle: 'sqlwarden.preference.sidebar_style',
} as const

const defaultPreferences: AppShellPreferences = {
  themeMode: 'system',
  contentLayout: 'centered',
  navbarStyle: 'scroll',
  sidebarStyle: 'sidebar',
}

export function useAppShellPreferences() {
  const { theme, setTheme } = useTheme()
  const [preferences, setPreferencesState] = useState<AppShellPreferences>(() => readPreferences(theme))

  useEffect(() => {
    applyPreferences(preferences)
  }, [preferences])

  useEffect(() => {
    setPreferencesState((current) => (
      current.themeMode === theme ? current : { ...current, themeMode: theme }
    ))
  }, [theme])

  const setPreferences: Dispatch<SetStateAction<AppShellPreferences>> = (nextPreferences) => {
    setPreferencesState((current) => {
      const resolvedPreferences = typeof nextPreferences === 'function'
        ? nextPreferences(current)
        : nextPreferences

      if (resolvedPreferences.themeMode !== current.themeMode) {
        setTheme(resolvedPreferences.themeMode)
      }

      return resolvedPreferences
    })
  }

  return { preferences, setPreferences }
}

export function AppShellHeader({
  label,
  icon,
  description,
}: {
  label: string
  icon: typeof User02Icon
  description?: string
}) {
  return (
    <SidebarHeader className="border-b border-sidebar-border">
      <div className="flex items-center gap-2">
        <SidebarMenu className="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
          <SidebarMenuItem>
            <SidebarMenuButton
              tooltip={label}
              className={description ? 'h-auto items-center py-2 hover:bg-transparent' : 'hover:bg-transparent'}
            >
              <div className="flex size-6 shrink-0 items-center justify-center bg-sidebar-primary text-sidebar-primary-foreground [&_svg]:size-3.5">
                <HugeiconsIcon icon={icon} strokeWidth={2.5} />
              </div>
              <span className="grid min-w-0 flex-1 gap-0.5 text-left">
                <span className="truncate font-semibold tracking-tight">{label}</span>
                {description ? (
                  <span className="truncate text-[11px] font-normal leading-none text-sidebar-foreground/50">{description}</span>
                ) : null}
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <SidebarTrigger
          className="shrink-0 cursor-pointer"
          aria-label="Toggle sidebar"
        />
      </div>
    </SidebarHeader>
  )
}

export function AppShellNavSection({
  label,
  items,
  pathname,
}: {
  label?: string
  items: AppShellNavItem[]
  pathname: string
}) {
  return (
    <div className="flex flex-col gap-1 px-2 py-1 group-data-[collapsible=icon]:px-2">
      {label ? (
        <div className="flex h-6 items-center gap-1.5 px-2 text-[10px] font-semibold tracking-widest text-sidebar-foreground/40 uppercase group-data-[collapsible=icon]:hidden">
          <span className="size-1 shrink-0 rounded-full bg-sidebar-foreground/30" />
          <span>{label}</span>
        </div>
      ) : null}
      <SidebarMenu>
        {items.map((item) => (
          <AppShellNavMenuItem key={navItemKey(item)} item={item} isActive={isNavItemActive(pathname, item)} />
        ))}
      </SidebarMenu>
    </div>
  )
}

export function AppShellSidebarFooter({
  session,
  preferences,
  setPreferences,
  extraUserItems = [],
}: {
  session: SessionResponse
  preferences: AppShellPreferences
  setPreferences: Dispatch<SetStateAction<AppShellPreferences>>
  extraUserItems?: AppShellNavItem[]
}) {
  return (
    <SidebarFooter className="border-t border-sidebar-border">
      <AppShellPreferencesPopover preferences={preferences} setPreferences={setPreferences} />
      <AppShellUserMenu session={session} extraItems={extraUserItems} />
    </SidebarFooter>
  )
}

export function AppShellRail() {
  return <SidebarRail />
}

export function AppShellContent({
  preferences,
  children,
}: {
  preferences: AppShellPreferences
  children: ReactNode
}) {
  return (
    <main className={preferences.contentLayout === 'centered' ? 'mx-auto min-h-svh w-full max-w-screen-2xl px-4 py-6 md:px-6' : 'min-h-svh px-4 py-6 md:px-6'}>
      {children}
    </main>
  )
}

function AppShellNavMenuItem({
  item,
  isActive,
}: {
  item: AppShellNavItem
  isActive: boolean
}) {
  return (
    <SidebarMenuItem>
      <div className="pointer-events-none absolute inset-y-0.5 left-0 w-0.5 bg-sidebar-primary opacity-0 transition-opacity peer-data-active/menu-button:opacity-100" />
      <SidebarMenuButton
        render={<Link to={item.to as never} params={item.params as never} />}
        isActive={isActive}
        tooltip={item.label}
      >
        <HugeiconsIcon icon={item.icon} strokeWidth={2} />
        <span>{item.label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function AppShellUserMenu({
  session,
  extraItems,
}: {
  session: SessionResponse
  extraItems: AppShellNavItem[]
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const logout = useMutation({
    mutationFn: async () => api.post<void>('/api/v1/auth/logout'),
    onSettled: async () => {
      clearAccessToken()
      clearAuthScopedQueryCache(queryClient)
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
            <InitialsAvatar value={session.account.name} className="rounded-lg" />
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
                  <InitialsAvatar value={session.account.name} className="rounded-lg" />
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
              {extraItems.map((item) => (
                <DropdownMenuItem
                  key={navItemKey(item)}
                  render={<Link to={item.to as never} params={item.params as never} />}
                >
                  <HugeiconsIcon icon={item.icon} strokeWidth={2} />
                  {item.label}
                </DropdownMenuItem>
              ))}
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

function AppShellPreferencesPopover({
  preferences,
  setPreferences,
}: {
  preferences: AppShellPreferences
  setPreferences: Dispatch<SetStateAction<AppShellPreferences>>
}) {
  function updatePreference<Key extends keyof AppShellPreferences>(key: Key, value: AppShellPreferences[Key]) {
    window.localStorage.setItem(preferenceKeys[key], value)
    setPreferences((current) => ({
      ...current,
      [key]: value,
    }))
  }

  function restoreDefaults() {
    Object.entries(preferenceKeys).forEach(([key, storageKey]) => {
      const typedKey = key as keyof AppShellPreferences
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
              onValueChange={(value) => updatePreference('themeMode', value as AppShellTheme)}
            />

            <PreferenceToggle
              label="Page Layout"
              value={preferences.contentLayout}
              options={['centered', 'full-width']}
              labels={{ centered: 'Centered', 'full-width': 'Full Width' }}
              onValueChange={(value) => updatePreference('contentLayout', value as AppShellContentLayout)}
            />

            <PreferenceToggle
              label="Navbar Behavior"
              value={preferences.navbarStyle}
              options={['sticky', 'scroll']}
              labels={{ sticky: 'Sticky', scroll: 'Scroll' }}
              onValueChange={(value) => updatePreference('navbarStyle', value as AppShellNavbarStyle)}
            />

            <PreferenceToggle
              label="Sidebar Style"
              value={preferences.sidebarStyle}
              options={['inset', 'sidebar', 'floating']}
              labels={{ inset: 'Inset', sidebar: 'Sidebar', floating: 'Floating' }}
              onValueChange={(value) => updatePreference('sidebarStyle', value as AppShellSidebarStyle)}
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

function readPreferences(themeMode: AppShellTheme): AppShellPreferences {
  return {
    themeMode,
    contentLayout: readPreference(preferenceKeys.contentLayout, ['centered', 'full-width'], defaultPreferences.contentLayout),
    navbarStyle: readPreference(preferenceKeys.navbarStyle, ['sticky', 'scroll'], defaultPreferences.navbarStyle),
    sidebarStyle: readPreference(preferenceKeys.sidebarStyle, ['sidebar', 'inset', 'floating'], defaultPreferences.sidebarStyle),
  }
}

function readPreference<Value extends string>(key: string, allowed: Value[], fallback: Value) {
  const stored = window.localStorage.getItem(key)
  return stored && allowed.includes(stored as Value) ? stored as Value : fallback
}

function applyPreferences(preferences: AppShellPreferences) {
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

function navItemKey(item: AppShellNavItem) {
  return `${item.to}:${JSON.stringify(item.params ?? {})}`
}

function isNavItemActive(pathname: string, item: AppShellNavItem) {
  const path = item.params
    ? Object.entries(item.params).reduce((nextPath, [key, value]) => nextPath.replace(`$${key}`, value), item.to)
    : item.to

  return trimTrailingSlash(pathname) === trimTrailingSlash(path)
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
