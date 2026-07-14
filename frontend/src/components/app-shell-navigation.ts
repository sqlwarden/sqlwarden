import type { AppIcon } from '#/lib/icons'
import { trimTrailingSlash } from '#/lib/utils'

export type AppShellNavItem = {
  to: string
  label: string
  icon: AppIcon
  params?: Record<string, string>
  search?: Record<string, unknown>
  disabled?: boolean
  badge?: string
  activePathPrefixes?: string[]
}

export function navItemKey(item: AppShellNavItem) {
  return `${item.to}:${JSON.stringify(item.params ?? {})}`
}

export function isNavItemActive(pathname: string, item: AppShellNavItem) {
  const normalizedPathname = trimTrailingSlash(pathname)
  const resolvedTo = resolveNavPath(item.to, item.params ?? {})

  if (normalizedPathname === trimTrailingSlash(resolvedTo)) return true

  return (
    item.activePathPrefixes?.some((prefix) => {
      const normalizedPrefix = trimTrailingSlash(prefix)
      return (
        normalizedPathname === normalizedPrefix ||
        normalizedPathname.startsWith(`${normalizedPrefix}/`)
      )
    }) ?? false
  )
}

export function resolveNavPath(to: string, params: Record<string, string>) {
  return Object.entries(params).reduce((path, [key, value]) => path.replace(`$${key}`, value), to)
}
