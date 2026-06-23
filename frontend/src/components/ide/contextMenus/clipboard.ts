import { toast } from 'sonner'
import type { SqlDialect } from '../sqlDialect'

/** Writes text to the clipboard, falling back to a hidden textarea when the
 *  async Clipboard API is unavailable (insecure context / older browsers). */
export function writeClipboard(text: string): void {
  try {
    if (navigator.clipboard) {
      void navigator.clipboard.writeText(text)
      return
    }
  } catch { /* fall through to legacy path */ }
  const el = document.createElement('textarea')
  el.value = text
  el.style.cssText = 'position:fixed;opacity:0'
  document.body.appendChild(el)
  el.select()
  try { document.execCommand('copy') } catch { /* ignore */ }
  document.body.removeChild(el)
}

/** Copies text and shows a brief confirmation toast. */
export function copyWithToast(text: string, label = 'Copied'): void {
  writeClipboard(text)
  toast.success(label)
}

/** Comma-separated identifier list (already dialect-formatted by the caller). */
export function columnList(names: string[]): string {
  return names.join(', ')
}

/** `table.column`, each part quoted per dialect only when its shape requires it. */
export function qualifiedColumn(dialect: SqlDialect, table: string, column: string): string {
  return `${dialect.formatColumn(table)}.${dialect.formatColumn(column)}`
}

/** A single result row as tab-separated values. */
export function rowToTsv(cells: string[]): string {
  return cells.join('\t')
}

/** A result row as a pretty-printed JSON object keyed by column name. */
export function rowToJson(columnNames: string[], cells: string[]): string {
  const obj: Record<string, string> = {}
  columnNames.forEach((name, i) => { obj[name] = cells[i] ?? '' })
  return JSON.stringify(obj, null, 2)
}

/** Multiple rows as newline-joined TSV lines. */
export function selectionToTsv(rows: string[][]): string {
  return rows.map((r) => r.join('\t')).join('\n')
}

/** A single column's values, one per line. */
export function valuesToLines(values: string[]): string {
  return values.join('\n')
}
