import type { EditorTab } from '../useIdeStore'

export const MAX_BROWSER_CSV_BYTES = 10 * 1024 * 1024

export function isCsvFileTab(tab: EditorTab | undefined): boolean {
  if (tab?.kind !== 'file') return false

  const mediaType = tab.fileMediaType?.split(';', 1)[0]?.trim().toLowerCase()
  return (
    mediaType === 'text/csv' ||
    tab.fileKind?.trim().toLowerCase() === 'csv' ||
    tab.title.toLowerCase().endsWith('.csv')
  )
}

export function isCsvFileTooLarge(tab: EditorTab | undefined): boolean {
  return (
    isCsvFileTab(tab) &&
    tab?.fileSizeBytes !== undefined &&
    tab.fileSizeBytes > MAX_BROWSER_CSV_BYTES
  )
}
