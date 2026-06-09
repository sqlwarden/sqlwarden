import { useEffect, useState } from 'react'
import type { Dispatch, ReactNode, SetStateAction } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Icon, useIconPack, type AppIcon } from '#/lib/icons'
import type { IconPackName } from '#/lib/icons'
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
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarTrigger,
} from '#/components/ui/sidebar'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { useEditorTheme } from '#/lib/editor-themes/context'
import { EDITOR_THEME_LABELS, VALID_EDITOR_THEMES } from '#/lib/editor-themes'
import type { EditorThemeName } from '#/lib/editor-themes'
import { useEditorFont } from '#/lib/editor-font/context'
import { EDITOR_FONTS, EDITOR_FONT_SIZES, DEFAULT_EDITOR_FONT, DEFAULT_EDITOR_FONT_SIZE } from '#/lib/editor-font/context'
import type { EditorFont, EditorFontSize } from '#/lib/editor-font/context'
import { Slider } from '#/components/ui/slider'

export type AppShellTheme = 'dark' | 'light' | 'system'
export type AppShellSidebarStyle = 'sidebar' | 'inset' | 'floating'

export type AppShellPreferences = {
  themeMode: AppShellTheme
  sidebarStyle: AppShellSidebarStyle
}

export type AppShellNavItem = {
  to: string
  label: string
  icon: AppIcon
  params?: Record<string, string>
  disabled?: boolean
  badge?: string
  activePathPrefixes?: string[]
}

const preferenceKeys = {
  themeMode: 'sqlwarden.preference.theme_mode',
  sidebarStyle: 'sqlwarden.preference.sidebar_style',
} as const

const defaultPreferences: AppShellPreferences = {
  themeMode: 'system',
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
  icon: AppIcon
  description?: string
}) {
  return (
    <SidebarHeader className="border-b border-sidebar-border">
      {/* Collapsed: show logo icon centred */}
      <div className="hidden items-center justify-center py-1 group-data-[collapsible=icon]:flex">
        <div className="flex size-8 shrink-0 items-center justify-center bg-sidebar-primary text-sidebar-primary-foreground [&_svg]:size-4">
          <Icon name={icon} size={16} />
        </div>
      </div>
      {/* Expanded: show full label + description */}
      <SidebarMenu className="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
        <SidebarMenuItem>
          <SidebarMenuButton
            tooltip={label}
            className={description ? 'h-auto items-center py-2 hover:bg-transparent' : 'hover:bg-transparent'}
          >
            <div className="flex size-6 shrink-0 items-center justify-center bg-sidebar-primary text-sidebar-primary-foreground [&_svg]:size-3.5">
              <Icon name={icon} size={14} />
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
      <div className="flex justify-center px-2 pb-1">
        <SidebarTrigger className="w-full cursor-pointer group-data-[collapsible=icon]:w-auto" aria-label="Toggle sidebar" />
      </div>
    </SidebarFooter>
  )
}

export function AppShellRail() {
  return <SidebarRail resizable />
}

export function AppShellContent({
  children,
}: {
  preferences?: AppShellPreferences
  children: ReactNode
}) {
  return (
    <main className="min-h-svh px-4 py-6 md:px-6">
      <div className="mb-4 flex md:hidden">
        <SidebarTrigger
          className="cursor-pointer border border-border bg-background shadow-sm"
          aria-label="Open navigation"
        />
      </div>
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
  if (item.disabled) {
    return (
      <SidebarMenuItem>
        <SidebarMenuButton
          disabled
          tooltip={item.label}
          className={item.badge ? 'pr-14' : undefined}
        >
          <Icon name={item.icon} size={20} />
          <span>{item.label}</span>
        </SidebarMenuButton>
        {item.badge ? <SidebarMenuBadge>{item.badge}</SidebarMenuBadge> : null}
      </SidebarMenuItem>
    )
  }

  return (
    <SidebarMenuItem>
      <div className="pointer-events-none absolute inset-y-0.5 left-0 w-0.5 bg-sidebar-primary opacity-0 transition-opacity peer-data-active/menu-button:opacity-100" />
      <SidebarMenuButton
        render={<Link to={item.to as never} params={item.params as never} />}
        isActive={isActive}
        tooltip={item.label}
      >
        <Icon name={item.icon} size={20} />
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
              <DropdownMenuItem render={<Link to="/settings/account" />}>
                <Icon name="settings-02" size={20} />
                Settings
              </DropdownMenuItem>
              {session.organizations.length >= 2 ? (
                <DropdownMenuItem render={<Link to="/settings/my-organizations" />}>
                  <Icon name="building-04" size={20} />
                  Switch Organization
                </DropdownMenuItem>
              ) : null}
              {extraItems.map((item) => (
                <DropdownMenuItem
                  key={navItemKey(item)}
                  render={<Link to={item.to as never} params={item.params as never} />}
                >
                  <Icon name={item.icon} size={20} />
                  {item.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              disabled={logout.isPending}
              onClick={() => {
                void logout.mutateAsync()
              }}
            >
              <Icon name="logout-03" size={20} />
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
  const { packName, setPackName } = useIconPack()
  const { editorThemeDark, editorThemeLight, setEditorThemeDark, setEditorThemeLight } = useEditorTheme()
  const { editorFont, editorFontSize, setEditorFont, setEditorFontSize } = useEditorFont()

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
    setPackName('hugeicons')
    setEditorThemeDark('vscode-dark')
    setEditorThemeLight('vscode-light')
    setEditorFont(DEFAULT_EDITOR_FONT)
    setEditorFontSize(DEFAULT_EDITOR_FONT_SIZE)
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
        <Icon name="paint-board" size={20} />
        <span className="group-data-[collapsible=icon]:hidden">UI Preferences</span>
      </PopoverTrigger>
      <PopoverContent side="right" align="end" className="w-80 max-h-[calc(100dvh-1rem)]">
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
            <div className="text-xs font-medium text-muted-foreground">App</div>
            <PreferenceToggle
              label="Theme Mode"
              value={preferences.themeMode}
              options={['light', 'dark', 'system']}
              onValueChange={(value) => updatePreference('themeMode', value as AppShellTheme)}
            />

            <PreferenceToggle
              label="Sidebar Style"
              value={preferences.sidebarStyle}
              options={['inset', 'sidebar', 'floating']}
              labels={{ inset: 'Inset', sidebar: 'Sidebar', floating: 'Floating' }}
              onValueChange={(value) => updatePreference('sidebarStyle', value as AppShellSidebarStyle)}
            />

            <PreferenceToggle
              label="Icon Pack"
              value={packName}
              options={['hugeicons', 'lucide', 'remix']}
              labels={{ hugeicons: 'HugeIcons', lucide: 'Lucide', remix: 'Remix' }}
              onValueChange={(value) => setPackName(value as IconPackName)}
            />
          </div>

          <div className="flex flex-col gap-3">
            <div className="text-xs font-medium text-muted-foreground">Editor</div>
            <EditorThemeSelect
              label="Dark Theme"
              value={editorThemeDark}
              onValueChange={setEditorThemeDark}
            />
            <EditorThemeSelect
              label="Light Theme"
              value={editorThemeLight}
              onValueChange={setEditorThemeLight}
            />
            <EditorFontSelect
              value={editorFont}
              onValueChange={setEditorFont}
            />
            <EditorFontSizeSlider
              value={editorFontSize}
              onValueChange={setEditorFontSize}
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

function EditorThemeSelect({
  label,
  value,
  onValueChange,
}: {
  label: string
  value: EditorThemeName
  onValueChange: (value: EditorThemeName) => void
}) {
  const items = VALID_EDITOR_THEMES.map((name) => ({
    label: EDITOR_THEME_LABELS[name],
    value: name,
  }))

  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">{label}</div>
      <Select
        items={items}
        value={value}
        onValueChange={(v) => v && onValueChange(v as EditorThemeName)}
      >
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {VALID_EDITOR_THEMES.map((name) => (
              <SelectItem key={name} value={name}>
                {EDITOR_THEME_LABELS[name]}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function EditorFontSelect({
  value,
  onValueChange,
}: {
  value: EditorFont
  onValueChange: (font: EditorFont) => void
}) {
  const items = EDITOR_FONTS.map((f) => ({ label: f.label, value: f.fontFamily }))

  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">Font</div>
      <Select
        items={items}
        value={value.fontFamily}
        onValueChange={(v) => {
          if (!v) return
          const found = EDITOR_FONTS.find((f) => f.fontFamily === v)
          if (found) onValueChange(found)
        }}
      >
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {EDITOR_FONTS.map((f) => (
              <SelectItem key={f.fontFamily} value={f.fontFamily}>
                {f.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function EditorFontSizeSlider({
  value,
  onValueChange,
}: {
  value: EditorFontSize
  onValueChange: (size: EditorFontSize) => void
}) {
  const min = EDITOR_FONT_SIZES[0]
  const max = EDITOR_FONT_SIZES[EDITOR_FONT_SIZES.length - 1]

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <div className="text-xs font-medium">Font Size</div>
        <div className="text-xs tabular-nums text-muted-foreground">{value}px</div>
      </div>
      <Slider
        value={[value]}
        min={min}
        max={max}
        step={1}
        onValueChange={(val) => {
          const v = Array.isArray(val) ? val[0] : val
          if (typeof v === 'number') onValueChange(v as EditorFontSize)
        }}
      />
    </div>
  )
}

function readPreferences(themeMode: AppShellTheme): AppShellPreferences {
  return {
    themeMode,
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
  root.setAttribute('data-content-layout', 'full-width')
  root.removeAttribute('data-navbar-style')
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
  const normalizedPathname = trimTrailingSlash(pathname)
  const resolvedTo = resolvePath(item.to, item.params ?? {})

  if (normalizedPathname === trimTrailingSlash(resolvedTo)) return true

  return (
    item.activePathPrefixes?.some((prefix) => {
      const normalizedPrefix = trimTrailingSlash(prefix)
      return normalizedPathname === normalizedPrefix || normalizedPathname.startsWith(`${normalizedPrefix}/`)
    }) ?? false
  )
}

function resolvePath(to: string, params: Record<string, string>) {
  return Object.entries(params).reduce((path, [key, value]) => path.replace(`$${key}`, value), to)
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
