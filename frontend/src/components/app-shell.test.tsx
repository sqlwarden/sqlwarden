import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SidebarProvider } from '#/components/ui/sidebar'
import { AppShellHeader } from './app-shell'

describe('AppShellHeader', () => {
  it('renders a named AppIcon by string', () => {
    render(
      <SidebarProvider>
        <AppShellHeader label="Settings" icon="settings-02" />
      </SidebarProvider>,
    )
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('renders a custom element icon such as a brand mark', () => {
    render(
      <SidebarProvider>
        <AppShellHeader label="SQLWarden" icon={<span data-testid="custom-icon" />} />
      </SidebarProvider>,
    )
    expect(screen.getAllByTestId('custom-icon')).toHaveLength(2)
  })
})
