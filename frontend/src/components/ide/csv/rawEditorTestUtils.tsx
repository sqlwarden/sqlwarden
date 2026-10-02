import type { PropsWithChildren } from 'react'
import { EditorView } from '@codemirror/view'
import { ThemeProvider } from '#/components/theme-provider'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { EditorThemeProvider } from '#/lib/editor-themes/context'

export function EditorProviders({ children }: PropsWithChildren) {
  return (
    <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
      <EditorThemeProvider>
        <EditorFontProvider>{children}</EditorFontProvider>
      </EditorThemeProvider>
    </ThemeProvider>
  )
}

export function editorViewOf(container: HTMLElement): EditorView {
  const dom = container.querySelector<HTMLElement>('.cm-editor')
  const view = dom && EditorView.findFromDOM(dom)
  if (!view) throw new Error('CodeMirror view not mounted')
  return view
}
