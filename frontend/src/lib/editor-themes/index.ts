import type { Extension } from '@codemirror/state'

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

export async function loadEditorTheme(name: EditorThemeName): Promise<Extension> {
  try {
    switch (name) {
      case 'sqlwarden-dark': {
        const { default: theme } = await import('./themes/sqlwarden-dark')
        return theme
      }
      case 'sqlwarden-light': {
        const { default: theme } = await import('./themes/sqlwarden-light')
        return theme
      }
      case 'vscode-dark': {
        const { vscodeDark } = await import('@uiw/codemirror-theme-vscode')
        return vscodeDark
      }
      case 'vscode-light': {
        const { vscodeLight } = await import('@uiw/codemirror-theme-vscode')
        return vscodeLight
      }
      case 'github-dark': {
        const { githubDark } = await import('@uiw/codemirror-theme-github')
        return githubDark
      }
      case 'github-light': {
        const { githubLight } = await import('@uiw/codemirror-theme-github')
        return githubLight
      }
      case 'dracula': {
        const { dracula } = await import('@uiw/codemirror-theme-dracula')
        return dracula
      }
      case 'darcula': {
        const { darcula } = await import('@uiw/codemirror-theme-darcula')
        return darcula
      }
      case 'gruvbox-dark': {
        const { gruvboxDark } = await import('@uiw/codemirror-theme-gruvbox-dark')
        return gruvboxDark
      }
      case 'gruvbox-light': {
        const { default: theme } = await import('./themes/gruvbox-light')
        return theme
      }
    }
  } catch {
    const { default: theme } = await import('./themes/sqlwarden-dark')
    return theme
  }
}
