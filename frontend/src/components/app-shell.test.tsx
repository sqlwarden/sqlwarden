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

  it('truncates a long label and wraps it in a hover tooltip trigger with the full text', () => {
    const longLabel = 'A Very Long Organization Name That Would Overflow The Sidebar Width'
    render(
      <SidebarProvider>
        <AppShellHeader label={longLabel} icon="settings-02" />
      </SidebarProvider>,
    )
    const label = screen.getByText(longLabel)
    expect(label).toHaveClass('truncate')
    expect(label.closest('[data-slot="tooltip-trigger"]')).not.toBeNull()
  })
})
