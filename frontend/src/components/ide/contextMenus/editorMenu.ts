import type { ContextMenuItem } from '#/components/ui/context-menu'
import { RUN_SHORTCUT, FORMAT_SHORTCUT } from '../IdeToolbar'

export type SqlEditorMenuCtx = {
  onCut: () => void
  onCopy: () => void
  onPaste: () => void
  onSelectAll: () => void
  /** Whether the tab is runnable SQL (console/scratch tab, or a .sql file) —
   *  gates the run/format section so a plain non-SQL file only gets edit ops. */
  isSqlTab: boolean
  canRun: boolean
  onRunStatement: () => void
  onRunAll: () => void
  /** Whether the active connection's engine implements EXPLAIN at all —
   *  disables (not hides) the Explain item when false. */
  canExplain: boolean
  /** Whether the active connection's dialect supports EXPLAIN ANALYZE —
   *  disables (not hides) the Explain Analyze item when false. */
  canExplainAnalyze: boolean
  onExplain: () => void
  onExplainAnalyze: () => void
  onFormat: () => void
  onSaveFavorite: () => void
}

export function buildSqlEditorMenu(ctx: SqlEditorMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'cut', label: 'Cut', icon: 'cut-01', onSelect: ctx.onCut },
    { kind: 'action', id: 'copy', label: 'Copy', icon: 'copy-01', onSelect: ctx.onCopy },
    { kind: 'action', id: 'paste', label: 'Paste', icon: 'paste-01', onSelect: ctx.onPaste },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'select-all',
      label: 'Select All',
      icon: 'select-all-01',
      onSelect: ctx.onSelectAll,
    },
  ]

  if (!ctx.isSqlTab) return items

  items.push(
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'run-statement',
      label: 'Run Statement',
      icon: 'play',
      shortcut: RUN_SHORTCUT,
      disabled: !ctx.canRun,
      disabledReason: ctx.canRun ? undefined : 'No connection',
      onSelect: ctx.onRunStatement,
    },
    {
      kind: 'action',
      id: 'run-all',
      label: 'Run All',
      icon: 'play',
      disabled: !ctx.canRun,
      disabledReason: ctx.canRun ? undefined : 'No connection',
      onSelect: ctx.onRunAll,
    },
    {
      kind: 'action',
      id: 'explain',
      label: 'Explain',
      icon: 'search-01',
      disabled: !ctx.canRun || !ctx.canExplain,
      disabledReason: !ctx.canRun
        ? 'No connection'
        : ctx.canExplain
          ? undefined
          : 'This connection does not support EXPLAIN',
      onSelect: ctx.onExplain,
    },
    {
      kind: 'action',
      id: 'explain-analyze',
      label: 'Explain Analyze',
      icon: 'search-01',
      disabled: !ctx.canRun || !ctx.canExplainAnalyze,
      disabledReason: !ctx.canRun
        ? 'No connection'
        : ctx.canExplainAnalyze
          ? undefined
          : 'This connection does not support EXPLAIN ANALYZE',
      onSelect: ctx.onExplainAnalyze,
    },
    {
      kind: 'action',
      id: 'format',
      label: 'Format SQL',
      icon: 'subject',
      shortcut: FORMAT_SHORTCUT,
      onSelect: ctx.onFormat,
    },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'save-favorite',
      label: 'Save as favorite',
      icon: 'star',
      onSelect: ctx.onSaveFavorite,
    },
  )

  return items
}
