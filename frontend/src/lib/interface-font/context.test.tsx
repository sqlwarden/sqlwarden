import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@fontsource-variable/geist', () => ({}))
vi.mock('@fontsource-variable/inter', () => ({}))
vi.mock('@fontsource-variable/ibm-plex-sans', () => ({}))
vi.mock('@fontsource-variable/manrope', () => ({}))
vi.mock('@fontsource-variable/space-grotesk', () => ({}))
vi.mock('@fontsource-variable/epilogue', () => ({}))
vi.mock('@fontsource-variable/mozilla-text', () => ({}))
vi.mock('@fontsource-variable/zalando-sans', () => ({}))
vi.mock('@fontsource-variable/zalando-sans/wght-italic.css', () => ({}))

import {
  DEFAULT_INTERFACE_FONT,
  INTERFACE_FONTS,
  InterfaceFontProvider,
  loadInterfaceFont,
  useInterfaceFont,
} from './context'

describe('interface font defaults', () => {
  it('defaults to Zalando Sans as the brand UI font', () => {
    expect(DEFAULT_INTERFACE_FONT.label).toBe('Zalando Sans')
    expect(INTERFACE_FONTS[0].label).toBe('Zalando Sans')
  })

  it('lazy-loads Inter now that it is no longer the eager default', async () => {
    const inter = INTERFACE_FONTS.find((f) => f.label === 'Inter')
    if (!inter) throw new Error('Inter option missing')
    await expect(loadInterfaceFont(inter)).resolves.toBeUndefined()
  })

  it('lazy-loads Geist now that it is no longer the eager default', async () => {
    const geist = INTERFACE_FONTS.find((f) => f.label === 'Geist')
    if (!geist) throw new Error('Geist option missing')
    await expect(loadInterfaceFont(geist)).resolves.toBeUndefined()
  })

  it('offers Cal Sans UI as a self-hosted option', async () => {
    const calSansUi = INTERFACE_FONTS.find((f) => f.label === 'Cal Sans UI')
    if (!calSansUi) throw new Error('Cal Sans UI option missing')
    await expect(loadInterfaceFont(calSansUi)).resolves.toBeUndefined()
  })

  it.each(['Glide', 'Mozilla Text', 'Zalando Sans'])('offers and loads %s', async (label) => {
    const font = INTERFACE_FONTS.find((option) => option.label === label)
    if (!font) throw new Error(`${label} option missing`)
    await expect(loadInterfaceFont(font)).resolves.toBeUndefined()
  })
})

describe('InterfaceFontProvider', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.style.removeProperty('--font-interface')
  })

  it('applies the default Zalando Sans stack to --font-interface on mount', async () => {
    const { result } = renderHook(() => useInterfaceFont(), { wrapper: InterfaceFontProvider })
    expect(result.current.interfaceFont.label).toBe('Zalando Sans')
    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--font-interface')).toBe(
        DEFAULT_INTERFACE_FONT.fontFamily,
      ),
    )
  })

  it('persists a selected font and applies it to the CSS custom property', async () => {
    const { result } = renderHook(() => useInterfaceFont(), { wrapper: InterfaceFontProvider })
    const geist = INTERFACE_FONTS.find((f) => f.label === 'Geist')
    if (!geist) throw new Error('Geist option missing')

    act(() => result.current.setInterfaceFont(geist))

    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--font-interface')).toBe(
        geist.fontFamily,
      ),
    )
    expect(localStorage.getItem('sqlwarden.preference.interface_font')).toBe(geist.fontFamily)
  })
})
