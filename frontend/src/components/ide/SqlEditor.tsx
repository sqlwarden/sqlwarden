import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { toast } from 'sonner'
import { EditorView } from '@codemirror/view'
import { EditorState, Compartment } from '@codemirror/state'
import type { Extension } from '@codemirror/state'
import { selectAll as selectAllCommand } from '@codemirror/commands'
import { yCollab } from 'y-codemirror.next'
import type * as Y from 'yjs'
import { cn } from '#/lib/utils'
import { ContextMenu } from '#/components/ui/context-menu'
import { useTheme } from '#/components/theme-provider'
import { useEditorTheme } from '#/lib/editor-themes/context'
import { loadEditorTheme, getCachedTheme } from '#/lib/editor-themes'
import { useEditorFont, loadEditorFont, editorFontSizeRem } from '#/lib/editor-font/context'
import type { EditorFontSize } from '#/lib/editor-font/context'
import { sqlwardenBasicSetup } from './codemirrorSetup'
import { findPanelHost, type FindPanelHost } from './findPanelBridge'
import { FindPanel } from './FindPanel'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useIde } from './useIdeStore'
import { sqlCompletionExtension, type SQLCompletionConfig } from './completion'
import { Icon, useIconPack } from '#/lib/icons'
import { sqlFormatterForDriver, sqlFormattingKeymap } from './sqlFormatting'
import { buildSqlEditorMenu } from './contextMenus/editorMenu'
import { readClipboardFallback, writeClipboard } from './contextMenus/clipboard'
import { statementHoverExtension, type HoveredStatement } from './statementHover'
import {
  statementPreviewHighlightExtension,
  setStatementPreview,
} from './statementPreviewHighlight'
import { sqlStatementWithOffsetsAtCursor } from './sqlStatements'

function makeBaseTheme(fontFamily: string, fontSize: EditorFontSize): Extension {
  return EditorView.theme({
    '&': { height: '100%' },
    '.cm-scroller': {
      fontFamily,
      fontSize: editorFontSizeRem(fontSize),
      lineHeight: '1.65',
      overflow: 'auto',
    },
    // Extra top padding gives the hover hint room to sit above the first
    // statement in the document, which has no preceding line to overlap into.
    '.cm-content': { padding: '20px 0 8px' },
    '.cm-lineNumbers .cm-gutterElement': { minWidth: '3.5ch', textAlign: 'right' },
    '.cm-foldGutter': { display: 'none' },
    '.cm-tooltip:not(.cm-tooltip-autocomplete)': { borderRadius: '0' },
    '.cm-statement-preview': {
      backgroundColor: 'color-mix(in srgb, var(--foreground) 6%, transparent)',
    },
  })
}

// ─── Component ─────────────────────────────────────────────────────────────────

export type SqlEditorContextMenuConfig = {
  isSqlTab: boolean
  canRun: boolean
  onRunStatement: () => void
  onRunAll: () => void
  canExplain: boolean
  canExplainAnalyze: boolean
  onExplain: () => void
  onExplainAnalyze: () => void
  onFormat: () => void
  onSaveFavorite: () => void
  onRunSegment: (sql: string) => void
  onExplainSegment: (sql: string) => void
}

type SqlEditorProps = {
  tabId: string
  /** Owning group id — distinguishes two panes showing the same tab in the view registry. */
  groupId?: string
  /** The Y.Doc backing this editor. Must have a Y.Text at key 'content'. */
  doc: Y.Doc
  className?: string
  onCursorChange?: (line: number, col: number, selSize: number, selectedText: string) => void
  completion?: SQLCompletionConfig
  driver?: string
  contextMenu?: SqlEditorContextMenuConfig
}

export function SqlEditor({
  tabId,
  groupId,
  doc,
  className,
  onCursorChange,
  completion,
  driver,
  contextMenu,
}: SqlEditorProps) {
  const viewKey = groupId ? `${groupId}:${tabId}` : tabId
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRegistry = useEditorViewRegistry()
  const onCursorChangeRef = useRef(onCursorChange)
  onCursorChangeRef.current = onCursorChange

  const { resolvedTheme } = useTheme()
  const { editorThemeDark, editorThemeLight } = useEditorTheme()
  const activeThemeName = resolvedTheme === 'dark' ? editorThemeDark : editorThemeLight
  const { editorFont, editorFontSize } = useEditorFont()
  const { iconMap } = useIconPack()
  const notifyConnectionRequired = useCallback(() => {
    toast.info('Select a connection for schema-aware autocomplete.', {
      id: 'sql-completion-connection-required',
      description: 'SQL keywords are still available without a connection.',
    })
  }, [])
  const initialAppearance = useRef({
    fontFamily: editorFont.fontFamily,
    fontSize: editorFontSize,
    themeName: activeThemeName,
  })

  const themeCompartment = useRef(new Compartment())
  const fontCompartment = useRef(new Compartment())
  const completionCompartment = useRef(new Compartment())
  const formattingCompartment = useRef(new Compartment())
  const completionConfig = useMemo(
    () => ({
      orgSlug: completion?.orgSlug,
      workspaceId: completion?.workspaceId,
      connectionId: completion?.connectionId,
      driver: completion?.driver,
      sessionId: completion?.sessionId,
      iconMap,
      onConnectionRequired: completion?.onConnectionRequired ?? notifyConnectionRequired,
    }),
    [
      completion?.orgSlug,
      completion?.workspaceId,
      completion?.connectionId,
      completion?.driver,
      completion?.sessionId,
      completion?.onConnectionRequired,
      iconMap,
      notifyConnectionRequired,
    ],
  )
  const initialCompletionConfig = useRef(completionConfig)
  const initialFormatter = useRef(sqlFormatterForDriver(driver))
  const notifyFormattingError = useCallback(() => {
    toast.error('Could not format SQL.', {
      description: 'The query may contain unsupported or incomplete syntax.',
    })
  }, [])
  const viewRef = useRef<EditorView | null>(null)
  const [findHost, setFindHost] = useState<FindPanelHost | null>(null)
  const [hoveredStatement, setHoveredStatement] = useState<HoveredStatement | null>(null)
  const [cursorStatement, setCursorStatement] = useState<HoveredStatement | null>(null)

  // Re-mount the editor whenever the active doc changes.
  // key={activeTab.id} at the call site also ensures clean remount on tab switch.
  useEffect(() => {
    if (!containerRef.current) return
    const yText = doc.getText('content')

    const view = new EditorView({
      state: EditorState.create({
        doc: yText.toString(),
        extensions: [
          sqlwardenBasicSetup,
          completionCompartment.current.of(sqlCompletionExtension(initialCompletionConfig.current)),
          formattingCompartment.current.of(
            sqlFormattingKeymap(initialFormatter.current, notifyFormattingError),
          ),
          fontCompartment.current.of(
            makeBaseTheme(initialAppearance.current.fontFamily, initialAppearance.current.fontSize),
          ),
          themeCompartment.current.of(getCachedTheme(initialAppearance.current.themeName) ?? []),
          findPanelHost.of(setFindHost),
          statementHoverExtension(setHoveredStatement),
          statementPreviewHighlightExtension(),
          EditorView.lineWrapping,
          yCollab(yText, null), // handles all CodeMirror ↔ Y.js sync
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              setHoveredStatement(null)
              setCursorStatement(null)
            }
            if (update.selectionSet) {
              const head = update.state.selection.main.head
              const statement = sqlStatementWithOffsetsAtCursor(update.state.doc.toString(), head)
              const coords = statement ? update.view.coordsAtPos(statement.start) : null
              if (statement && coords) {
                const editorRect = update.view.dom.getBoundingClientRect()
                setCursorStatement({
                  sql: statement.sql,
                  start: statement.start,
                  end: statement.end,
                  top: coords.top - editorRect.top,
                  left: coords.left - editorRect.left,
                })
              } else {
                setCursorStatement(null)
              }
            }
            if (!update.selectionSet && !update.docChanged) return
            const cb = onCursorChangeRef.current
            if (!cb) return
            const { from, to } = update.state.selection.main
            const head = update.state.selection.main.head
            const line = update.state.doc.lineAt(head)
            cb(line.number, head - line.from + 1, to - from, update.state.sliceDoc(from, to))
          }),
        ],
      }),
      parent: containerRef.current,
    })

    viewRef.current = view
    viewRegistry.register(viewKey, view)

    return () => {
      viewRef.current = null
      viewRegistry.unregister(viewKey)
      view.destroy()
    }
  }, [doc, viewKey, viewRegistry, notifyFormattingError])

  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: completionCompartment.current.reconfigure(sqlCompletionExtension(completionConfig)),
    })
  }, [completionConfig])

  useEffect(() => {
    if (!viewRef.current) return
    viewRef.current.dispatch({
      effects: formattingCompartment.current.reconfigure(
        sqlFormattingKeymap(sqlFormatterForDriver(driver), notifyFormattingError),
      ),
    })
  }, [driver, notifyFormattingError])

  // Grab keyboard focus when this pane is the target of a split, so the new
  // split is ready to type in. requestAnimationFrame waits for layout.
  const focusEditorRequest = useIde((s) => s.focusEditorRequest)
  const setFocusEditorRequest = useIde((s) => s.setFocusEditorRequest)
  useEffect(() => {
    if (focusEditorRequest !== viewKey) return
    requestAnimationFrame(() => viewRef.current?.focus())
    setFocusEditorRequest(null)
  }, [focusEditorRequest, viewKey, setFocusEditorRequest])

  // Apply a line/column jump requested by search. Compared against the raw
  // tabId (not viewKey) — a jump targets a file's tab and should apply
  // regardless of which split group's pane currently shows it.
  const pendingJump = useIde((s) => s.pendingJump)
  const clearPendingJump = useIde((s) => s.clearPendingJump)
  useEffect(() => {
    if (!pendingJump || pendingJump.tabId !== tabId) return
    const view = viewRef.current
    if (!view) return
    const targetLine = Math.min(Math.max(pendingJump.line, 1), view.state.doc.lines)
    const docLine = view.state.doc.line(targetLine)
    const column = Math.max(0, pendingJump.column - 1)
    const pos = Math.min(docLine.from + column, docLine.to)
    view.dispatch({
      selection: { anchor: pos },
      effects: EditorView.scrollIntoView(pos, { y: 'center' }),
    })
    view.focus()
    clearPendingJump()
  }, [pendingJump, tabId, clearPendingJump])

  // Hot-swap the theme without remounting the editor.
  useEffect(() => {
    let cancelled = false
    loadEditorTheme(activeThemeName).then((ext) => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({
        effects: themeCompartment.current.reconfigure(ext),
      })
    })
    return () => {
      cancelled = true
    }
  }, [activeThemeName])

  // Hot-swap font / font-size without remounting.
  // Load font CSS first (no-op for system fonts and already-loaded web fonts).
  useEffect(() => {
    let cancelled = false
    loadEditorFont(editorFont).then(() => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({
        effects: fontCompartment.current.reconfigure(
          makeBaseTheme(editorFont.fontFamily, editorFontSize),
        ),
      })
    })
    return () => {
      cancelled = true
    }
  }, [editorFont, editorFontSize])

  const handleCut = useCallback(() => {
    const view = viewRef.current
    if (!view) return
    const selection = view.state.selection.main
    if (selection.empty) return
    writeClipboard(view.state.sliceDoc(selection.from, selection.to))
    view.dispatch({ changes: { from: selection.from, to: selection.to, insert: '' } })
    view.focus()
  }, [])

  const handleCopy = useCallback(() => {
    const view = viewRef.current
    if (!view) return
    const selection = view.state.selection.main
    if (selection.empty) return
    writeClipboard(view.state.sliceDoc(selection.from, selection.to))
  }, [])

  const handlePaste = useCallback(() => {
    const view = viewRef.current
    if (!view) return

    function insert(text: string) {
      const current = viewRef.current
      if (!current || !text) return
      const selection = current.state.selection.main
      current.dispatch({
        changes: { from: selection.from, to: selection.to, insert: text },
        selection: { anchor: selection.from + text.length },
      })
      current.focus()
    }

    function pasteViaFallback() {
      const text = readClipboardFallback()
      if (text) {
        insert(text)
        return
      }
      toast.error('Could not paste.', {
        description:
          'Clipboard access requires HTTPS (or localhost) in this browser. Use Ctrl/Cmd+V instead.',
      })
    }

    // The Clipboard API only exists in secure contexts (https, or localhost) —
    // on a plain-http LAN/dev host `navigator.clipboard` is undefined and we
    // fall back to the legacy execCommand path below.
    if (!navigator.clipboard) {
      view.focus()
      pasteViaFallback()
      return
    }

    // The context menu closing steals document focus for a frame; reading the
    // clipboard while unfocused throws "Document is not focused" in Chromium.
    // Refocus the editor first and defer the read to the next frame so focus
    // has actually landed before the async Clipboard API call runs.
    view.focus()
    requestAnimationFrame(() => {
      navigator.clipboard
        .readText()
        .then(insert)
        .catch(() => {
          view.focus()
          pasteViaFallback()
        })
    })
  }, [])

  const handleSelectAll = useCallback(() => {
    const view = viewRef.current
    if (!view) return
    selectAllCommand(view)
    view.focus()
  }, [])

  const menuItems = useMemo(
    () =>
      buildSqlEditorMenu({
        onCut: handleCut,
        onCopy: handleCopy,
        onPaste: handlePaste,
        onSelectAll: handleSelectAll,
        isSqlTab: contextMenu?.isSqlTab ?? false,
        canRun: contextMenu?.canRun ?? false,
        onRunStatement: contextMenu?.onRunStatement ?? (() => {}),
        onRunAll: contextMenu?.onRunAll ?? (() => {}),
        canExplain: contextMenu?.canExplain ?? false,
        canExplainAnalyze: contextMenu?.canExplainAnalyze ?? false,
        onExplain: contextMenu?.onExplain ?? (() => {}),
        onExplainAnalyze: contextMenu?.onExplainAnalyze ?? (() => {}),
        onFormat: contextMenu?.onFormat ?? (() => {}),
        onSaveFavorite: contextMenu?.onSaveFavorite ?? (() => {}),
      }),
    [handleCut, handleCopy, handlePaste, handleSelectAll, contextMenu],
  )

  const activeStatement = hoveredStatement ?? cursorStatement
  const showHoverActions = contextMenu?.isSqlTab && contextMenu.canRun && activeStatement

  return (
    <>
      <ContextMenu
        items={menuItems}
        className="relative h-full overflow-hidden"
        onMouseLeave={() => {
          setHoveredStatement(null)
          viewRef.current?.dispatch({ effects: setStatementPreview.of(null) })
        }}
      >
        <div
          ref={containerRef}
          className={cn('h-full overflow-hidden', className)}
          onBlur={() => setCursorStatement(null)}
        />
        {showHoverActions && (
          <div
            className="absolute z-10 flex -translate-y-full items-center gap-2 rounded-sm bg-card px-1 text-[11px] leading-none whitespace-nowrap"
            style={{ top: activeStatement.top, left: activeStatement.left }}
            onMouseEnter={() => {
              viewRef.current?.dispatch({
                effects: setStatementPreview.of({
                  from: activeStatement.start,
                  to: activeStatement.end,
                }),
              })
            }}
            onMouseLeave={() => {
              viewRef.current?.dispatch({ effects: setStatementPreview.of(null) })
            }}
          >
            <button
              type="button"
              className="flex items-center gap-1 text-muted-foreground hover:text-foreground hover:underline"
              onClick={() => {
                contextMenu.onRunSegment(activeStatement.sql)
                setHoveredStatement(null)
                setCursorStatement(null)
                viewRef.current?.dispatch({ effects: setStatementPreview.of(null) })
              }}
            >
              <Icon name="play" size={10} />
              Run
            </button>
            {contextMenu.canExplain && (
              <button
                type="button"
                className="flex items-center gap-1 text-muted-foreground hover:text-foreground hover:underline"
                onClick={() => {
                  contextMenu.onExplainSegment(activeStatement.sql)
                  setHoveredStatement(null)
                  setCursorStatement(null)
                  viewRef.current?.dispatch({ effects: setStatementPreview.of(null) })
                }}
              >
                <Icon name="subject" size={10} />
                Explain
              </button>
            )}
          </div>
        )}
      </ContextMenu>
      {findHost && createPortal(<FindPanel view={findHost.view} />, findHost.dom)}
    </>
  )
}
