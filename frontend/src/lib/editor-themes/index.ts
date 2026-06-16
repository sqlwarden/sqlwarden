import type { Extension } from '@codemirror/state'

// Module-level cache so getCachedTheme() returns synchronously after first load.
// This lets SqlEditor seed the Compartment at mount time and avoid a flash of
// the default CM styles before the hot-swap effect fires.
const _cache = new Map<EditorThemeName, Extension>()

export function getCachedTheme(name: EditorThemeName): Extension | undefined {
  return _cache.get(name)
}

export type EditorThemeName =
  | 'sqlwarden-dark'
  | 'sqlwarden-light'
  | 'vscode-dark'
  | 'vscode-light'
  | 'github-dark'
  | 'github-light'
  | 'dracula'
  | 'darcula'
  | 'gruvbox-dark'
  | 'gruvbox-light'

export const EDITOR_THEME_LABELS: Record<EditorThemeName, string> = {
  'sqlwarden-dark':  'SQLWarden Dark',
  'sqlwarden-light': 'SQLWarden Light',
  'vscode-dark':     'VS Code Dark',
  'vscode-light':    'VS Code Light',
  'github-dark':     'GitHub Dark',
  'github-light':    'GitHub Light',
  'dracula':         'Dracula',
  'darcula':         'Darcula',
  'gruvbox-dark':    'Gruvbox Dark',
  'gruvbox-light':   'Gruvbox Light',
}

export const VALID_EDITOR_THEMES = Object.keys(EDITOR_THEME_LABELS) as EditorThemeName[]

async function _loadFresh(name: EditorThemeName): Promise<Extension> {
  switch (name) {
    case 'sqlwarden-dark':  return (await import('./themes/sqlwarden-dark')).default
    case 'sqlwarden-light': return (await import('./themes/sqlwarden-light')).default
    // Drop the VS Code themes' baked-in fontFamily so the user's editor font
    // setting applies, the same as every other theme. createTheme only emits a
    // font rule when settings.fontFamily is truthy.
    case 'vscode-dark':     return (await import('@uiw/codemirror-theme-vscode')).vscodeDarkInit({ settings: { fontFamily: '' } })
    case 'vscode-light':    return (await import('@uiw/codemirror-theme-vscode')).vscodeLightInit({ settings: { fontFamily: '' } })
    case 'github-dark':     return (await import('@uiw/codemirror-theme-github')).githubDark
    case 'github-light':    return (await import('@uiw/codemirror-theme-github')).githubLight
    case 'dracula':         return (await import('@uiw/codemirror-theme-dracula')).dracula
    case 'darcula':         return (await import('@uiw/codemirror-theme-darcula')).darcula
    case 'gruvbox-dark':    return (await import('@uiw/codemirror-theme-gruvbox-dark')).gruvboxDark
    case 'gruvbox-light':   return (await import('./themes/gruvbox-light')).default
  }
}

export async function loadEditorTheme(name: EditorThemeName): Promise<Extension> {
  const cached = _cache.get(name)
  if (cached) return cached
  try {
    const ext = await _loadFresh(name)
    _cache.set(name, ext)
    return ext
  } catch {
    const fallback = (await import('./themes/sqlwarden-dark')).default
    return fallback
  }
}
