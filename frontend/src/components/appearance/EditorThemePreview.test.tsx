import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ThemeProvider } from '#/components/theme-provider'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { EditorThemePreview } from './EditorThemePreview'

function renderPreview() {
  return render(
    <ThemeProvider disableTransitionOnChange={false}>
      <EditorThemeProvider>
        <EditorFontProvider>
          <EditorThemePreview />
        </EditorFontProvider>
      </EditorThemeProvider>
    </ThemeProvider>,
  )
}

describe('EditorThemePreview', () => {
  it('renders a labelled, read-only preview region with the sample query text', async () => {
    renderPreview()
    const region = await screen.findByRole('img', { name: /editor theme preview/i })
    expect(region).toBeInTheDocument()
    expect(region.textContent).toContain('SELECT')
  })
})
