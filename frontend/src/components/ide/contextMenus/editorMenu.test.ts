import { describe, it, expect } from 'vitest'
import type { ContextMenuItem, ContextMenuActionItem } from '#/components/ui/context-menu'
import { buildSqlEditorMenu } from './editorMenu'

const noop = () => {}

function action(items: ContextMenuItem[], id: string): ContextMenuActionItem | undefined {
  return items.find((i): i is ContextMenuActionItem => i.kind === 'action' && i.id === id)
}

describe('buildSqlEditorMenu', () => {
  const base = {
    onCut: noop,
    onCopy: noop,
    onPaste: noop,
    onSelectAll: noop,
    onRunStatement: noop,
    onRunAll: noop,
    onFormat: noop,
    onSaveFavorite: noop,
  }

  it('always includes the edit ops', () => {
    const items = buildSqlEditorMenu({ ...base, isSqlTab: false, canRun: false })
    expect(action(items, 'cut')?.onSelect).toBe(base.onCut)
    expect(action(items, 'copy')?.onSelect).toBe(base.onCopy)
    expect(action(items, 'paste')?.onSelect).toBe(base.onPaste)
    expect(action(items, 'select-all')?.onSelect).toBe(base.onSelectAll)
  })

  it('omits the run/format section for a non-SQL tab', () => {
    const items = buildSqlEditorMenu({ ...base, isSqlTab: false, canRun: true })
    expect(action(items, 'run-statement')).toBeUndefined()
    expect(action(items, 'run-all')).toBeUndefined()
    expect(action(items, 'format')).toBeUndefined()
  })

  it('includes run/format for a SQL tab, enabled when a connection is selected', () => {
    const items = buildSqlEditorMenu({ ...base, isSqlTab: true, canRun: true })
    expect(action(items, 'run-statement')?.disabled).toBeFalsy()
    expect(action(items, 'run-all')?.disabled).toBeFalsy()
    expect(action(items, 'format')?.onSelect).toBe(base.onFormat)
  })

  it('disables run actions with a reason when there is no connection', () => {
    const items = buildSqlEditorMenu({ ...base, isSqlTab: true, canRun: false })
    expect(action(items, 'run-statement')?.disabled).toBe(true)
    expect(action(items, 'run-statement')?.disabledReason).toBe('No connection')
    expect(action(items, 'run-all')?.disabled).toBe(true)
  })

  it('wires save-favorite to the real handler', () => {
    const items = buildSqlEditorMenu({ ...base, isSqlTab: true, canRun: true })
    expect(action(items, 'save-favorite')?.soon).toBeFalsy()
    expect(action(items, 'save-favorite')?.onSelect).toBe(base.onSaveFavorite)
  })

  it('does not offer export — that lives in the main toolbar', () => {
    const items = buildSqlEditorMenu({ ...base, isSqlTab: true, canRun: true })
    expect(action(items, 'export')).toBeUndefined()
  })
})
