import { createContext, useContext, useState } from 'react'
import type { ReactNode } from 'react'
import { VALID_EDITOR_THEMES } from './index'
import type { EditorThemeName } from './index'

const DARK_KEY  = 'sqlwarden.preference.editor_theme_dark'
const LIGHT_KEY = 'sqlwarden.preference.editor_theme_light'

function readPref(key: string, fallback: EditorThemeName): EditorThemeName {
  const stored = localStorage.getItem(key)
  return stored && VALID_EDITOR_THEMES.includes(stored as EditorThemeName)
    ? (stored as EditorThemeName)
    : fallback
}

type EditorThemeContextValue = {
  editorThemeDark:     EditorThemeName
  editorThemeLight:    EditorThemeName
  setEditorThemeDark:  (name: EditorThemeName) => void
  setEditorThemeLight: (name: EditorThemeName) => void
}

const EditorThemeContext = createContext<EditorThemeContextValue>({
  editorThemeDark:    'sqlwarden-dark',
  editorThemeLight:   'sqlwarden-light',
  setEditorThemeDark:  () => {},
  setEditorThemeLight: () => {},
})

export function EditorThemeProvider({ children }: { children: ReactNode }) {
  const [dark,  setDarkState]  = useState<EditorThemeName>(() => readPref(DARK_KEY,  'sqlwarden-dark'))
  const [light, setLightState] = useState<EditorThemeName>(() => readPref(LIGHT_KEY, 'sqlwarden-light'))

  function setEditorThemeDark(name: EditorThemeName) {
    localStorage.setItem(DARK_KEY, name)
    setDarkState(name)
  }

  function setEditorThemeLight(name: EditorThemeName) {
    localStorage.setItem(LIGHT_KEY, name)
    setLightState(name)
  }

  return (
    <EditorThemeContext.Provider value={{ editorThemeDark: dark, editorThemeLight: light, setEditorThemeDark, setEditorThemeLight }}>
      {children}
    </EditorThemeContext.Provider>
  )
}

export function useEditorTheme() {
  return useContext(EditorThemeContext)
}
