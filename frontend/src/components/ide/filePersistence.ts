import type { EditorTab } from './useIdeStore'

export type SaveEditorTabDependencies = {
  readContent: (tab: EditorTab) => string
  updateFile: (fileId: number, content: string, etag: string) => Promise<{ etag: string }>
  updateTabEtag: (tabId: string, etag: string) => void
}

export type SaveEditorTabResult = { kind: 'saved' } | { kind: 'save-as'; tab: EditorTab }

/** Saves an existing file or returns a content-current tab for Save As. */
export async function saveEditorTab(
  tab: EditorTab,
  dependencies: SaveEditorTabDependencies,
): Promise<SaveEditorTabResult> {
  const content = dependencies.readContent(tab)
  if (tab.kind !== 'file' || !tab.etag || !tab.fileId) {
    return { kind: 'save-as', tab: { ...tab, content } }
  }

  const result = await dependencies.updateFile(tab.fileId, content, tab.etag)
  dependencies.updateTabEtag(tab.id, result.etag)
  return { kind: 'saved' }
}
