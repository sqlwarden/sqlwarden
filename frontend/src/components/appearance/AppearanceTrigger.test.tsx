import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { ThemeProvider } from '#/components/theme-provider'
import { ThemeLabProvider } from '#/lib/theme-lab/context'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { HeadingFontProvider } from '#/lib/heading-font/context'
import { InterfaceFontProvider } from '#/lib/interface-font/context'
import { AppearanceTrigger } from './AppearanceTrigger'

function renderTrigger() {
  return render(
    <ThemeProvider disableTransitionOnChange={false}>
      <ThemeLabProvider>
        <EditorThemeProvider>
          <EditorFontProvider>
            <HeadingFontProvider>
              <InterfaceFontProvider>
                <AppearanceTrigger buttonLabel="Appearance" />
              </InterfaceFontProvider>
            </HeadingFontProvider>
          </EditorFontProvider>
        </EditorThemeProvider>
      </ThemeLabProvider>
    </ThemeProvider>,
  )
}

describe('AppearanceTrigger', () => {
  it('opens the Appearance dialog on click', async () => {
    const user = userEvent.setup()
    renderTrigger()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Appearance' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toBeInTheDocument()
    expect(screen.getByText('UI Scale')).toBeInTheDocument()
  })
})
