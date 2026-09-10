import { screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '#/test/render'
import { ThemeProvider, useTheme } from './theme-provider'

afterEach(() => {
  delete window.go
})

function ThemeHarness() {
  const { setTheme } = useTheme()
  return <button onClick={() => setTheme('dark')}>Use dark theme</button>
}

describe('ThemeProvider desktop integration', () => {
  it('synchronizes initial and changed themes with native window chrome', async () => {
    const setTheme = vi.fn(async () => undefined)
    window.go = {
      main: {
        DesktopBridge: {
          StartSession: vi.fn(),
          GetInfo: vi.fn(),
          RevealDataDirectory: vi.fn(),
          RevealLogDirectory: vi.fn(),
          SetTheme: setTheme,
        },
      },
    }
    const { user } = renderWithProviders(
      <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
        <ThemeHarness />
      </ThemeProvider>,
    )

    await waitFor(() => expect(setTheme).toHaveBeenCalledWith('light', 'light'))
    await user.click(screen.getByRole('button', { name: 'Use dark theme' }))
    await waitFor(() => expect(setTheme).toHaveBeenLastCalledWith('dark', 'dark'))
  })
})
