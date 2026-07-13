import type { YDocRegistry } from './useYDocRegistry'
import type { EditorTab } from './useIdeStore'

export type FileContentPayload = { text: string; etag: string }

/** Applies initial server content without overwriting local or peer Y.js state. */
export function hydrateFileTab(
  tab: EditorTab,
  payload: FileContentPayload,
  registry: Pick<YDocRegistry, 'getOrCreate'>,
  updateTabEtag: (tabId: string, etag: string) => void,
) {
  if (tab.isDirty && tab.ySnapshot) return

  const doc = registry.getOrCreate(tab.id)
  const text = doc.getText('content')
  if (text.length === 0) {
    doc.transact(() => text.insert(0, payload.text), 'server-load')
  }
  if (tab.etag === undefined) updateTabEtag(tab.id, payload.etag)
}
