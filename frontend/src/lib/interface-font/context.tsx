import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'

export type InterfaceFont = {
  label: string
  fontFamily: string
}

export const INTERFACE_FONTS: InterfaceFont[] = [
  // Geist is the app default — loaded globally via styles.css.
  { label: 'Geist',          fontFamily: "'Geist Variable', 'Inter Variable', system-ui, sans-serif" },
  // Inter is also loaded globally (long-time previous default, common fallback).
  { label: 'Inter',          fontFamily: "'Inter Variable', system-ui, sans-serif" },
  // @fontsource-variable packages register under the "Variable" family name.
  { label: 'IBM Plex Sans',  fontFamily: "'IBM Plex Sans Variable', system-ui, sans-serif" },
  { label: 'Manrope',        fontFamily: "'Manrope Variable', system-ui, sans-serif" },
  { label: 'Space Grotesk',  fontFamily: "'Space Grotesk Variable', system-ui, sans-serif" },
  { label: 'Epilogue',       fontFamily: "'Epilogue Variable', system-ui, sans-serif" },
  { label: 'System Font',    fontFamily: 'system-ui, sans-serif' },
]

export const DEFAULT_INTERFACE_FONT = INTERFACE_FONTS[0]

// Module-level cache — once a font's CSS is injected it persists in the document.
const _loadedFonts = new Set<string>()

export async function loadInterfaceFont(font: InterfaceFont): Promise<void> {
  if (_loadedFonts.has(font.fontFamily)) return
  _loadedFonts.add(font.fontFamily)
  switch (font.label) {
    case 'IBM Plex Sans': await import('@fontsource-variable/ibm-plex-sans'); break
    case 'Manrope':       await import('@fontsource-variable/manrope');       break
    case 'Space Grotesk': await import('@fontsource-variable/space-grotesk'); break
    case 'Epilogue':      await import('@fontsource-variable/epilogue');      break
    // Geist, Inter: loaded globally in styles.css. System Font: no web font.
  }
}

const FONT_KEY = 'sqlwarden.preference.interface_font'

function readFont(): InterfaceFont {
  const stored = localStorage.getItem(FONT_KEY)
  if (stored) {
    const found = INTERFACE_FONTS.find((f) => f.fontFamily === stored)
    if (found) return found
  }
  return DEFAULT_INTERFACE_FONT
}

type InterfaceFontContextValue = {
  interfaceFont: InterfaceFont
  setInterfaceFont: (font: InterfaceFont) => void
}

const InterfaceFontContext = createContext<InterfaceFontContextValue>({
  interfaceFont: DEFAULT_INTERFACE_FONT,
  setInterfaceFont: () => {},
})

export function InterfaceFontProvider({ children }: { children: ReactNode }) {
  const [font, setFontState] = useState<InterfaceFont>(() => readFont())

  // Apply through the --font-interface slot that styles.css routes font-sans to,
  // after the font CSS is present so there is no flash of fallback metrics.
  useEffect(() => {
    let cancelled = false
    void loadInterfaceFont(font).then(() => {
      if (cancelled) return
      document.documentElement.style.setProperty('--font-interface', font.fontFamily)
    })
    return () => { cancelled = true }
  }, [font])

  function setInterfaceFont(f: InterfaceFont) {
    localStorage.setItem(FONT_KEY, f.fontFamily)
    setFontState(f)
  }

  return (
    <InterfaceFontContext.Provider value={{ interfaceFont: font, setInterfaceFont }}>
      {children}
    </InterfaceFontContext.Provider>
  )
}

export function useInterfaceFont() {
  return useContext(InterfaceFontContext)
}
