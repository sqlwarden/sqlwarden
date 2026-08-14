import { useState } from 'react'
import type { Dispatch, ReactElement, ReactNode, SetStateAction } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Icon, type AppIcon } from '#/lib/icons'
import type { SessionResponse } from '#/lib/api/types'
import { api } from '#/lib/api/client'
import { clearAccessToken } from '#/lib/auth/access-token'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { UserAvatar } from '#/components/UserAvatar'
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
import { Tip } from '#/components/ide/schema-diagram/Tip'
import { sectionCaptionClass } from '#/lib/typography'
import { buildUserMenuItems } from '#/lib/user-menu'
import { cn } from '#/lib/utils'
import {
  useAppShellPreferences,
  type AppShellPreferences,
  type AppShellSidebarStyle,
  type AppShellTheme,
} from './app-shell-preferences'
import { isNavItemActive, navItemKey, type AppShellNavItem } from './app-shell-navigation'
import { UiLabPanel } from './ui-lab-panel'

export { useAppShellPreferences }
export type { AppShellPreferences, AppShellNavItem, AppShellSidebarStyle, AppShellTheme }

export function AppShellHeader({ label, icon }: { label: string; icon: AppIcon | ReactElement }) {
  const iconNode = typeof icon === 'string' ? <Icon name={icon} size={18} /> : icon

  return (
    <SidebarHeader className="border-b border-sidebar-border">
      {/* Collapsed: show the mono mark centred */}
      <div className="hidden items-center justify-center py-2.5 text-sidebar-foreground group-data-[collapsible=icon]:flex [&_svg]:size-5">
        {iconNode}
      </div>
      {/* Expanded: mono mark + name; name gets a hover tooltip since long names truncate */}
      <SidebarMenu className="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
        <SidebarMenuItem>
          <Tip label={label} side="right">
            <SidebarMenuButton className="h-auto items-center gap-2.5 py-2.5 hover:bg-transparent">
              <span className="shrink-0 text-sidebar-foreground [&_svg]:size-5">{iconNode}</span>
              <span className="min-w-0 flex-1 truncate text-left font-heading text-[15px] font-semibold tracking-tight">
                {label}
              </span>
            </SidebarMenuButton>
          </Tip>
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
        <div
          className={cn(
            sectionCaptionClass,
            'flex h-6 items-center gap-1.5 px-2 text-sidebar-foreground/40 group-data-[collapsible=icon]:hidden',
          )}
        >
          <span className="size-1 shrink-0 rounded-full bg-sidebar-foreground/30" />
          <span>{label}</span>
        </div>
      ) : null}
      <SidebarMenu>
        {items.map((item) => (
          <AppShellNavMenuItem
            key={navItemKey(item)}
            item={item}
            isActive={isNavItemActive(pathname, item)}
          />
        ))}
      </SidebarMenu>
    </div>
  )
}

export function AppShellSidebarFooter({
  session,
  preferences,
  setPreferences,
  hideUserMenu = false,
}: {
  session: SessionResponse
  preferences: AppShellPreferences
  setPreferences: Dispatch<SetStateAction<AppShellPreferences>>
  hideUserMenu?: boolean
}) {
  return (
    <SidebarFooter className="border-t border-sidebar-border">
      <AppShellPreferencesPopover
        preferences={preferences}
        setPreferences={setPreferences}
        isAdmin={session.is_instance_admin}
      />
      {hideUserMenu ? null : <AppShellUserMenu session={session} />}
      <div className="flex justify-center px-2 pb-1">
        <SidebarTrigger
          className="w-full cursor-pointer group-data-[collapsible=icon]:w-auto"
          aria-label="Toggle sidebar"
        />
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

function AppShellNavMenuItem({ item, isActive }: { item: AppShellNavItem; isActive: boolean }) {
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
        render={
          <Link to={item.to as never} params={item.params as never} search={item.search as never} />
        }
        isActive={isActive}
        tooltip={item.label}
      >
        <Icon name={item.icon} size={20} />
        <span>{item.label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

export function AppShellUserMenu({ session }: { session: SessionResponse }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const menuItems = buildUserMenuItems({ session })

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
            <UserAvatar value={session.account.name} />
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{session.account.name}</span>
              <span className="truncate text-xs text-muted-foreground">
                {session.account.email}
              </span>
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
                  <UserAvatar value={session.account.name} />
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium text-foreground">
                      {session.account.name}
                    </span>
                    <span className="truncate text-xs text-muted-foreground">
                      {session.account.email}
                    </span>
                  </div>
                </div>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              {menuItems.map((item) => (
                <DropdownMenuItem
                  key={item.id}
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
                logout.mutate()
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

export function AppShellPreferencesPopover({
  preferences,
  setPreferences,
  isAdmin,
  buttonLabel = 'UI Lab',
  buttonClassName,
}: {
  preferences: AppShellPreferences
  setPreferences: Dispatch<SetStateAction<AppShellPreferences>>
  isAdmin: boolean
  buttonLabel?: string
  buttonClassName?: string
}) {
  const [open, setOpen] = useState(false)

  const trigger = (
    <Button
      type="button"
      variant="ghost"
      aria-label={buttonLabel || 'UI Lab'}
      aria-pressed={open}
      onClick={() => setOpen((v) => !v)}
      className={cn(
        'w-full justify-start gap-2 group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0',
        open && 'bg-muted text-foreground',
        buttonClassName,
      )}
    >
      <Icon name="paint-board" size={20} />
      {buttonLabel ? (
        <span className="group-data-[collapsible=icon]:hidden">{buttonLabel}</span>
      ) : null}
    </Button>
  )

  return (
    <>
      {buttonLabel ? trigger : <Tip label="UI Lab">{trigger}</Tip>}
      {open && (
        <UiLabPanel
          preferences={preferences}
          setPreferences={setPreferences}
          onClose={() => setOpen(false)}
          isAdmin={isAdmin}
        />
      )}
    </>
  )
}
