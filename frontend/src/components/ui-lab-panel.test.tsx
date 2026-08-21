import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ThemeProvider } from '#/components/theme-provider'
import { UiLabPanel } from './ui-lab-panel'
import { defaultAppShellPreferences } from './app-shell-preferences'

describe('UiLabPanel', () => {
  it('shows only the Theme Mode toggle for non-admin users', () => {
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <UiLabPanel
          preferences={defaultAppShellPreferences}
          setPreferences={() => {}}
          onClose={() => {}}
          isAdmin={false}
        />
      </ThemeProvider>,
    )
    expect(screen.getByText('Theme Mode')).toBeInTheDocument()
    expect(screen.queryByText('Accent')).not.toBeInTheDocument()
    expect(screen.queryByText('Surface')).not.toBeInTheDocument()
    expect(screen.queryByText('Border Radius')).not.toBeInTheDocument()
  })

  it('shows the full customization surface for instance admins', () => {
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <UiLabPanel
          preferences={defaultAppShellPreferences}
          setPreferences={() => {}}
          onClose={() => {}}
          isAdmin
        />
      </ThemeProvider>,
    )
    expect(screen.getByText('Accent')).toBeInTheDocument()
    expect(screen.getByText('Surface')).toBeInTheDocument()
    expect(screen.getByText('Border Radius')).toBeInTheDocument()
  })
})
