import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { ThemeProvider } from '#/components/theme-provider'
import { ThemeLabProvider } from '#/lib/theme-lab/context'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { HeadingFontProvider } from '#/lib/heading-font/context'
import { InterfaceFontProvider } from '#/lib/interface-font/context'
import { AppearancePanel } from './AppearancePanel'

function renderPanel(isDev: boolean) {
  return render(
    <ThemeProvider disableTransitionOnChange={false}>
      <ThemeLabProvider>
        <EditorThemeProvider>
          <EditorFontProvider>
            <HeadingFontProvider>
              <InterfaceFontProvider>
                <AppearancePanel isDev={isDev} />
              </InterfaceFontProvider>
            </HeadingFontProvider>
          </EditorFontProvider>
        </EditorThemeProvider>
      </ThemeLabProvider>
    </ThemeProvider>,
  )
}

describe('AppearancePanel', () => {
  it('renders the production sections and no dev-only controls when isDev is false', () => {
    renderPanel(false)
    expect(screen.getByText('Theme')).toBeInTheDocument()
    expect(screen.getByText('Surface')).toBeInTheDocument()
    expect(screen.getByText('UI Scale')).toBeInTheDocument()
    expect(screen.getByText('Editor')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /editor theme preview/i })).toBeInTheDocument()
    expect(screen.queryByText('Accent')).not.toBeInTheDocument()
    expect(screen.queryByText('Border Radius')).not.toBeInTheDocument()
    expect(screen.queryByText(/Icon Pack/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Connections/i)).not.toBeInTheDocument()
  })

  it('exposes the dev-only controls after expanding the developer section when isDev is true', async () => {
    const user = userEvent.setup()
    renderPanel(true)
    expect(screen.getByText('Developer options')).toBeInTheDocument()
    expect(screen.queryByText('Accent')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /developer options/i }))

    expect(await screen.findByText('Accent')).toBeInTheDocument()
    expect(screen.getByText('Border Radius')).toBeInTheDocument()
  })

  it('changes the theme mode when a mode is picked', async () => {
    const user = userEvent.setup()
    renderPanel(false)
    await user.click(screen.getByRole('button', { name: /^Dark$/i }))
    expect(document.documentElement).toHaveClass('dark')
  })
})
