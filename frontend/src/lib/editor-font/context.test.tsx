import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@fontsource/geist-mono', () => ({}))
vi.mock('@fontsource-variable/fira-code', () => ({}))
vi.mock('@fontsource/cascadia-code', () => ({}))
vi.mock('@fontsource-variable/source-code-pro', () => ({}))
vi.mock('@fontsource-variable/roboto-mono', () => ({}))

import {
  DEFAULT_EDITOR_FONT,
  EDITOR_FONTS,
  EditorFontProvider,
  loadEditorFont,
  useEditorFont,
} from './context'

describe('editor font defaults', () => {
  it('defaults to JetBrains Mono as the brand data/code font', () => {
    expect(DEFAULT_EDITOR_FONT.label).toBe('JetBrains Mono')
    expect(EDITOR_FONTS[0].label).toBe('JetBrains Mono')
  })

  it('lazy-loads Geist Mono now that it is no longer the eager default', async () => {
    const geistMono = EDITOR_FONTS.find((f) => f.label === 'Geist Mono')
    if (!geistMono) throw new Error('Geist Mono option missing')
    await expect(loadEditorFont(geistMono)).resolves.toBeUndefined()
  })
})

describe('EditorFontProvider', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.style.removeProperty('--font-data')
  })

  it('applies the default JetBrains Mono stack to --font-data on mount', async () => {
    const { result } = renderHook(() => useEditorFont(), { wrapper: EditorFontProvider })
    expect(result.current.editorFont.label).toBe('JetBrains Mono')
    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--font-data')).toBe(
        DEFAULT_EDITOR_FONT.fontFamily,
      ),
    )
  })

  it('persists a selected font and applies it to the CSS custom property', async () => {
    const { result } = renderHook(() => useEditorFont(), { wrapper: EditorFontProvider })
    const geistMono = EDITOR_FONTS.find((f) => f.label === 'Geist Mono')
    if (!geistMono) throw new Error('Geist Mono option missing')

    act(() => result.current.setEditorFont(geistMono))

    await waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--font-data')).toBe(
        geistMono.fontFamily,
      ),
    )
    expect(localStorage.getItem('sqlwarden.preference.editor_font')).toBe(geistMono.fontFamily)
  })
})
