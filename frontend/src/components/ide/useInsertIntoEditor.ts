import { useCallback, useContext } from 'react'
import { IdeStoreContext, activeTabId as selectActiveTabId } from './useIdeStore'
import { useEditorViewRegistry } from './useEditorViewRegistry'

/** Returns insert(text): inserts text at the cursor of the focused editor pane,
 *  replacing any selection. No-op when no editor is focused/open. */
export function useInsertIntoEditor(): (text: string) => void {
  const store = useContext(IdeStoreContext)
  const viewRegistry = useEditorViewRegistry()

  return useCallback(
    (text: string) => {
      if (!store) return
      const s = store.getState()
      const ws = s.activeWorkspaceId
      if (ws === undefined) return
      const groupId = s.activeGroupId[ws]
      const tabId = selectActiveTabId(s, ws)
      if (!tabId) return
      const view = viewRegistry.get(groupId ? `${groupId}:${tabId}` : tabId)
      if (!view) return
      view.dispatch(view.state.replaceSelection(text))
      view.focus()
    },
    [store, viewRegistry],
  )
}
