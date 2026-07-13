import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  applyAppShellPreferences,
  readAppShellPreferences,
  useAppShellPreferences,
} from './app-shell-preferences'

const theme = vi.hoisted(() => ({ setTheme: vi.fn(), value: 'dark' as const }))
vi.mock('#/components/theme-provider', () => ({
  useTheme: () => ({ theme: theme.value, setTheme: theme.setTheme }),
}))

describe('app shell preferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.documentElement.removeAttribute('data-sidebar-variant')
  })

  it('reads only supported persisted values', () => {
    localStorage.setItem('sqlwarden.preference.sidebar_style', 'floating')
    expect(readAppShellPreferences('light')).toEqual({ themeMode: 'light', sidebarStyle: 'floating' })

    localStorage.setItem('sqlwarden.preference.sidebar_style', 'unknown')
    expect(readAppShellPreferences('system')).toEqual({ themeMode: 'system', sidebarStyle: 'sidebar' })
  })

  it('applies the supported shell attributes and removes obsolete attributes', () => {
    document.documentElement.setAttribute('data-theme-preset', 'old')
    document.documentElement.setAttribute('data-navbar-style', 'sticky')
    applyAppShellPreferences({ themeMode: 'dark', sidebarStyle: 'inset' })

    expect(document.documentElement).toHaveAttribute('data-theme-mode', 'dark')
    expect(document.documentElement).toHaveAttribute('data-content-layout', 'full-width')
    expect(document.documentElement).toHaveAttribute('data-sidebar-variant', 'inset')
    expect(document.documentElement).toHaveAttribute('data-sidebar-collapsible', 'icon')
    expect(document.documentElement).not.toHaveAttribute('data-theme-preset')
    expect(document.documentElement).not.toHaveAttribute('data-navbar-style')
  })

  it('keeps theme context and shell preferences synchronized', async () => {
    const { result } = renderHook(() => useAppShellPreferences())
    expect(result.current.preferences.themeMode).toBe('dark')

    act(() => result.current.setPreferences((current) => ({ ...current, themeMode: 'light', sidebarStyle: 'floating' })))
    expect(theme.setTheme).toHaveBeenCalledWith('light')
    await waitFor(() => expect(document.documentElement).toHaveAttribute('data-sidebar-variant', 'floating'))
  })
})
