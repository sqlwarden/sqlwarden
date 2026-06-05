import { createContext, useContext, useState } from 'react'
import type { ReactNode } from 'react'

export type EditorFont = {
  label: string
  fontFamily: string
}

export const EDITOR_FONTS: EditorFont[] = [
  // Roboto Mono is the app default — already loaded globally via styles.css
  { label: 'Roboto Mono',      fontFamily: "'Roboto Mono Variable', ui-monospace, monospace" },
  { label: 'System Font',      fontFamily: 'ui-monospace, monospace' },
  // @fontsource-variable packages register under the "Variable" family name
  { label: 'JetBrains Mono',   fontFamily: "'JetBrains Mono Variable', ui-monospace, monospace" },
  { label: 'Fira Code',        fontFamily: "'Fira Code Variable', ui-monospace, monospace" },
  { label: 'Cascadia Code',    fontFamily: "'Cascadia Code', ui-monospace, monospace" },
  { label: 'Source Code Pro',  fontFamily: "'Source Code Pro Variable', ui-monospace, monospace" },
  { label: 'Courier New',      fontFamily: "'Courier New', monospace" },
]

export const EDITOR_FONT_SIZES = [11, 12, 13, 14, 15, 16] as const
export type EditorFontSize = (typeof EDITOR_FONT_SIZES)[number]

export const DEFAULT_EDITOR_FONT      = EDITOR_FONTS[0]
export const DEFAULT_EDITOR_FONT_SIZE: EditorFontSize = 13

// Module-level cache — once a font's CSS is injected it persists in the document.
const _loadedFonts = new Set<string>()

export async function loadEditorFont(font: EditorFont): Promise<void> {
  if (_loadedFonts.has(font.fontFamily)) return
  _loadedFonts.add(font.fontFamily)
  switch (font.label) {
    case 'JetBrains Mono':  await import('@fontsource-variable/jetbrains-mono'); break
    case 'Fira Code':       await import('@fontsource-variable/fira-code');       break
    case 'Cascadia Code':   await import('@fontsource/cascadia-code');            break
    case 'Source Code Pro': await import('@fontsource-variable/source-code-pro'); break
    // Roboto Mono: loaded globally in styles.css — no lazy load needed.
    // System Font, Courier New: no web font required.
  }
}

const FONT_KEY      = 'sqlwarden.preference.editor_font'
const FONT_SIZE_KEY = 'sqlwarden.preference.editor_font_size'

function readFont(): EditorFont {
  const stored = localStorage.getItem(FONT_KEY)
  if (stored) {
    const found = EDITOR_FONTS.find((f) => f.fontFamily === stored)
    if (found) return found
  }
  return DEFAULT_EDITOR_FONT
}

function readFontSize(): EditorFontSize {
  const stored = localStorage.getItem(FONT_SIZE_KEY)
  if (stored) {
    const n = parseInt(stored, 10) as EditorFontSize
    if ((EDITOR_FONT_SIZES as readonly number[]).includes(n)) return n
  }
  return DEFAULT_EDITOR_FONT_SIZE
}

type EditorFontContextValue = {
  editorFont:        EditorFont
  editorFontSize:    EditorFontSize
  setEditorFont:     (font: EditorFont) => void
  setEditorFontSize: (size: EditorFontSize) => void
}

const EditorFontContext = createContext<EditorFontContextValue>({
  editorFont:        DEFAULT_EDITOR_FONT,
  editorFontSize:    DEFAULT_EDITOR_FONT_SIZE,
  setEditorFont:     () => {},
  setEditorFontSize: () => {},
})

export function EditorFontProvider({ children }: { children: ReactNode }) {
  const [font, setFontState] = useState<EditorFont>(() => readFont())
  const [size, setSizeState] = useState<EditorFontSize>(() => readFontSize())

  function setEditorFont(f: EditorFont) {
    localStorage.setItem(FONT_KEY, f.fontFamily)
    setFontState(f)
  }

  function setEditorFontSize(s: EditorFontSize) {
    localStorage.setItem(FONT_SIZE_KEY, String(s))
    setSizeState(s)
  }

  return (
    <EditorFontContext.Provider value={{ editorFont: font, editorFontSize: size, setEditorFont, setEditorFontSize }}>
      {children}
    </EditorFontContext.Provider>
  )
}

export function useEditorFont() {
  return useContext(EditorFontContext)
}
