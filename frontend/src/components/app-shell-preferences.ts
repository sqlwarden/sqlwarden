import { useEffect, useState, type Dispatch, type SetStateAction } from 'react'
import { useTheme } from '#/components/theme-provider'

export type AppShellTheme = 'dark' | 'light' | 'system'
export type AppShellSidebarStyle = 'sidebar' | 'inset' | 'floating'

export type AppShellPreferences = {
  themeMode: AppShellTheme
  sidebarStyle: AppShellSidebarStyle
}

export const appShellPreferenceKeys = {
  themeMode: 'sqlwarden.preference.theme_mode',
  sidebarStyle: 'sqlwarden.preference.sidebar_style',
} as const

export const defaultAppShellPreferences: AppShellPreferences = {
  themeMode: 'system',
  sidebarStyle: 'sidebar',
}

export function useAppShellPreferences() {
  const { theme, setTheme } = useTheme()
  const [preferences, setPreferencesState] = useState<AppShellPreferences>(() => readAppShellPreferences(theme))

  useEffect(() => {
    applyAppShellPreferences(preferences)
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

export function readAppShellPreferences(themeMode: AppShellTheme): AppShellPreferences {
  return {
    themeMode,
    sidebarStyle: readPreference(
      appShellPreferenceKeys.sidebarStyle,
      ['sidebar', 'inset', 'floating'],
      defaultAppShellPreferences.sidebarStyle,
    ),
  }
}

export function applyAppShellPreferences(preferences: AppShellPreferences) {
  const root = document.documentElement
  root.setAttribute('data-theme-mode', preferences.themeMode)
  root.removeAttribute('data-theme-preset')
  root.removeAttribute('data-font')
  root.setAttribute('data-content-layout', 'full-width')
  root.removeAttribute('data-navbar-style')
  root.setAttribute('data-sidebar-variant', preferences.sidebarStyle)
  root.setAttribute('data-sidebar-collapsible', 'icon')
}

function readPreference<Value extends string>(key: string, allowed: Value[], fallback: Value) {
  const stored = window.localStorage.getItem(key)
  return stored && allowed.includes(stored as Value) ? stored as Value : fallback
}
