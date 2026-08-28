import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'

export type HeadingFont = {
  label: string
  fontFamily: string
}

export const HEADING_FONTS: HeadingFont[] = [
  // Satoshi is the brand default — self-hosted via hand-written @font-face
  // rules in styles.css, same pattern as the other self-hosted faces below.
  { label: 'Satoshi', fontFamily: "'Satoshi', 'Geist Variable', system-ui, sans-serif" },
  // Cal Sans Heading: also self-hosted; a distinctive display face for
  // headings.
  {
    label: 'Cal Sans Heading',
    fontFamily: "'Cal Sans Heading', 'Satoshi', system-ui, sans-serif",
  },
  { label: 'Geist', fontFamily: "'Geist Variable', 'Inter Variable', system-ui, sans-serif" },
  { label: 'Inter', fontFamily: "'Inter Variable', system-ui, sans-serif" },
  // @fontsource-variable packages register under the "Variable" family name.
  { label: 'IBM Plex Sans', fontFamily: "'IBM Plex Sans Variable', system-ui, sans-serif" },
  { label: 'Manrope', fontFamily: "'Manrope Variable', system-ui, sans-serif" },
  { label: 'Space Grotesk', fontFamily: "'Space Grotesk Variable', system-ui, sans-serif" },
  { label: 'Epilogue', fontFamily: "'Epilogue Variable', system-ui, sans-serif" },
  { label: 'System Font', fontFamily: 'system-ui, sans-serif' },
]

export const DEFAULT_HEADING_FONT = HEADING_FONTS[0]

// Module-level cache — once a font's CSS is injected it persists in the document.
const _loadedFonts = new Set<string>()

export async function loadHeadingFont(font: HeadingFont): Promise<void> {
  if (_loadedFonts.has(font.fontFamily)) return
  _loadedFonts.add(font.fontFamily)
  switch (font.label) {
    case 'Geist':
      await import('@fontsource-variable/geist')
      break
    case 'IBM Plex Sans':
      await import('@fontsource-variable/ibm-plex-sans')
      break
    case 'Manrope':
      await import('@fontsource-variable/manrope')
      break
    case 'Space Grotesk':
      await import('@fontsource-variable/space-grotesk')
      break
    case 'Epilogue':
      await import('@fontsource-variable/epilogue')
      break
    // Satoshi, Cal Sans Heading: self-hosted via @font-face in styles.css.
    // Inter: loaded globally in styles.css. System Font: no web font.
  }
}

const FONT_KEY = 'sqlwarden.preference.heading_font'

function readFont(): HeadingFont {
  const stored = localStorage.getItem(FONT_KEY)
  if (stored) {
    const found = HEADING_FONTS.find((f) => f.fontFamily === stored)
    if (found) return found
  }
  return DEFAULT_HEADING_FONT
}

type HeadingFontContextValue = {
  headingFont: HeadingFont
  setHeadingFont: (font: HeadingFont) => void
}

const HeadingFontContext = createContext<HeadingFontContextValue>({
  headingFont: DEFAULT_HEADING_FONT,
  setHeadingFont: () => {},
})

export function HeadingFontProvider({ children }: { children: ReactNode }) {
  const [font, setFontState] = useState<HeadingFont>(() => readFont())

  // Apply through the --font-heading-face slot that styles.css routes
  // font-heading to, after the font CSS is present so there is no flash of
  // fallback metrics.
  useEffect(() => {
    let cancelled = false
    void loadHeadingFont(font).then(() => {
      if (cancelled) return
      document.documentElement.style.setProperty('--font-heading-face', font.fontFamily)
    })
    return () => {
      cancelled = true
    }
  }, [font])

  function setHeadingFont(f: HeadingFont) {
    localStorage.setItem(FONT_KEY, f.fontFamily)
    setFontState(f)
  }

  return (
    <HeadingFontContext.Provider value={{ headingFont: font, setHeadingFont }}>
      {children}
    </HeadingFontContext.Provider>
  )
}

export function useHeadingFont() {
  return useContext(HeadingFontContext)
}
