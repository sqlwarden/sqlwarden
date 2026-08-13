import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@fontsource-variable/geist', () => ({}))
vi.mock('@fontsource-variable/ibm-plex-sans', () => ({}))
vi.mock('@fontsource-variable/manrope', () => ({}))
vi.mock('@fontsource-variable/space-grotesk', () => ({}))
vi.mock('@fontsource-variable/epilogue', () => ({}))

import {
  DEFAULT_HEADING_FONT,
  HEADING_FONTS,
  HeadingFontProvider,
  loadHeadingFont,
  useHeadingFont,
} from './context'

describe('heading font defaults', () => {
  it('defaults to Satoshi as the brand heading font', () => {
    expect(DEFAULT_HEADING_FONT.label).toBe('Satoshi')
    expect(HEADING_FONTS[0].label).toBe('Satoshi')
  })

  it('offers Cal Sans Heading as a self-hosted option', async () => {
    const calSansHeading = HEADING_FONTS.find((f) => f.label === 'Cal Sans Heading')
    if (!calSansHeading) throw new Error('Cal Sans Heading option missing')
    await expect(loadHeadingFont(calSansHeading)).resolves.toBeUndefined()
  })

  it('lazy-loads Geist now that it is no longer the eager default', async () => {
    const geist = HEADING_FONTS.find((f) => f.label === 'Geist')
    if (!geist) throw new Error('Geist option missing')
    await expect(loadHeadingFont(geist)).resolves.toBeUndefined()
  })
})

describe('HeadingFontProvider', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.style.removeProperty('--font-heading-face')
  })

  it('applies the default Satoshi stack to --font-heading-face on mount', async () => {
    const { result } = renderHook(() => useHeadingFont(), { wrapper: HeadingFontProvider })
    expect(result.current.headingFont.label).toBe('Satoshi')
    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--font-heading-face')).toBe(
        DEFAULT_HEADING_FONT.fontFamily,
      ),
    )
  })

  it('persists a selected font and applies it to the CSS custom property', async () => {
    const { result } = renderHook(() => useHeadingFont(), { wrapper: HeadingFontProvider })
    const geist = HEADING_FONTS.find((f) => f.label === 'Geist')
    if (!geist) throw new Error('Geist option missing')

    act(() => result.current.setHeadingFont(geist))

    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--font-heading-face')).toBe(
        geist.fontFamily,
      ),
    )
    expect(localStorage.getItem('sqlwarden.preference.heading_font')).toBe(geist.fontFamily)
  })
})
