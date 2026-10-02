import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import {
  drawSelection,
  EditorView,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
} from '@codemirror/view'
import { yCollab } from 'y-codemirror.next'
import type * as Y from 'yjs'
import { cn } from '#/lib/utils'
import { useTheme } from '#/components/theme-provider'
import { useEditorTheme } from '#/lib/editor-themes/context'
import { getCachedTheme, loadEditorTheme } from '#/lib/editor-themes'
import { editorFontSizeRem, loadEditorFont, useEditorFont } from '#/lib/editor-font/context'
import type { EditorFontSize } from '#/lib/editor-font/context'
import { sqlwardenFindPanel } from '../codemirrorSetup'
import { FindPanel } from '../FindPanel'
import { findPanelHost, type FindPanelHost } from '../findPanelBridge'

export interface CsvRawViewState {
  anchor: number
  head: number
  scroll: ReturnType<EditorView['scrollSnapshot']>
}

function baseTheme(fontFamily: string, fontSize: EditorFontSize): Extension {
  return EditorView.theme({
    '&': { height: '100%' },
    '.cm-scroller': {
      fontFamily,
      fontSize: editorFontSizeRem(fontSize),
      lineHeight: '1.65',
      overflow: 'auto',
    },
    '.cm-content': { padding: '8px 0' },
    '.cm-lineNumbers .cm-gutterElement': { minWidth: '3.5ch', textAlign: 'right' },
  })
}

interface CsvRawEditorProps {
  /** Must have a Y.Text at key 'content'. */
  doc: Y.Doc
  /** Holds cursor and scroll across remounts, e.g. when toggling Table and Raw. */
  viewState: RefObject<CsvRawViewState | null>
  className?: string
}

export function CsvRawEditor({ doc, viewState, className }: CsvRawEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const [findHost, setFindHost] = useState<FindPanelHost | null>(null)
  const themeCompartment = useRef(new Compartment())
  const fontCompartment = useRef(new Compartment())

  const { resolvedTheme } = useTheme()
  const { editorThemeDark, editorThemeLight } = useEditorTheme()
  const activeThemeName = resolvedTheme === 'dark' ? editorThemeDark : editorThemeLight
  const { editorFont, editorFontSize } = useEditorFont()
  const initialAppearance = useRef({
    fontFamily: editorFont.fontFamily,
    fontSize: editorFontSize,
    themeName: activeThemeName,
  })

  // useLayoutEffect so cleanup runs while the container is still attached;
  // scrollSnapshot() needs live layout measurements.
  useLayoutEffect(() => {
    if (!containerRef.current) return
    const yText = doc.getText('content')
    const text = yText.toString()
    const restored = viewState.current
    const view = new EditorView({
      state: EditorState.create({
        doc: text,
        selection: restored
          ? {
              anchor: Math.min(restored.anchor, text.length),
              head: Math.min(restored.head, text.length),
            }
          : undefined,
        extensions: [
          lineNumbers(),
          highlightActiveLineGutter(),
          highlightSpecialChars(),
          history(),
          drawSelection(),
          highlightSelectionMatches(),
          sqlwardenFindPanel,
          findPanelHost.of(setFindHost),
          keymap.of([...defaultKeymap, ...searchKeymap, ...historyKeymap]),
          fontCompartment.current.of(
            baseTheme(initialAppearance.current.fontFamily, initialAppearance.current.fontSize),
          ),
          themeCompartment.current.of(getCachedTheme(initialAppearance.current.themeName) ?? []),
          yCollab(yText, null),
        ],
      }),
      parent: containerRef.current,
      scrollTo: restored?.scroll,
    })
    viewRef.current = view

    return () => {
      viewState.current = {
        anchor: view.state.selection.main.anchor,
        head: view.state.selection.main.head,
        scroll: view.scrollSnapshot(),
      }
      viewRef.current = null
      view.destroy()
    }
  }, [doc, viewState])

  useEffect(() => {
    let cancelled = false
    loadEditorTheme(activeThemeName).then((ext) => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({ effects: themeCompartment.current.reconfigure(ext) })
    })
    return () => {
      cancelled = true
    }
  }, [activeThemeName])

  useEffect(() => {
    let cancelled = false
    loadEditorFont(editorFont).then(() => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({
        effects: fontCompartment.current.reconfigure(
          baseTheme(editorFont.fontFamily, editorFontSize),
        ),
      })
    })
    return () => {
      cancelled = true
    }
  }, [editorFont, editorFontSize])

  return (
    <>
      <div
        ref={containerRef}
        aria-label="Raw CSV"
        className={cn('h-full overflow-hidden', className)}
      />
      {findHost && createPortal(<FindPanel view={findHost.view} />, findHost.dom)}
    </>
  )
}
