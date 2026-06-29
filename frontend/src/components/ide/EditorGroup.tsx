import { useMemo } from 'react'
import * as Y from 'yjs'
import { cn } from '#/lib/utils'
import type { Workspace } from '#/lib/api/types'
import { useIde } from './useIdeStore'
import type { GroupNode } from './ideLayout'
import { IdeTabBar } from './IdeTabBar'
import { SqlEditor } from './SqlEditor'
import { useYDocRegistry } from './useYDocRegistry'
import { useFileContent } from './useFileContent'
import { ObjectDetailView } from './object-detail/ObjectDetailView'

type EditorGroupProps = {
  orgSlug: string
  workspace: Workspace
  group: GroupNode
  focused: boolean
  /** Whether to render the focus ring (only meaningful when split into multiple groups). */
  showFocus?: boolean
  onCursorChange?: (line: number, col: number, sel: number) => void
}

export function EditorGroup({ orgSlug, workspace, group, focused, showFocus, onCursorChange }: EditorGroupProps) {
  const registry = useYDocRegistry()
  const tabs = useIde((s) => s.tabs)
  const focusGroup = useIde((s) => s.focusGroup)
  const updateTabEtag = useIde((s) => s.updateTabEtag)

  const activeTab = useMemo(() => tabs.find((t) => t.id === group.activeTabId), [tabs, group.activeTabId])

  const { isLoading, isError } = useFileContent({
    orgSlug,
    workspaceId: workspace.id,
    tab: activeTab,
    updateTabEtag,
  })

  const isObject = activeTab?.kind === 'object'

  // Populate the Y.Doc synchronously in render so SqlEditor mounts with content
  // (React runs child effects before parent effects, so deferring is too late).
  // Object tabs are not editors and never get a Y.Doc.
  let doc: Y.Doc | undefined
  if (activeTab && !isObject) {
    const initState = activeTab.ySnapshot ?? activeTab.yState
    const initialContent = !initState && activeTab.kind !== 'file' ? activeTab.content : undefined
    doc = registry.getOrCreate(activeTab.id, initialContent)
    if (initState && doc.getText('content').length === 0) {
      Y.applyUpdate(doc, new Uint8Array(initState), 'init')
    }
  }

  return (
    <div
      className={cn('flex h-full min-h-0 flex-col', showFocus && focused && 'ring-1 ring-inset ring-primary/25')}
      onMouseDownCapture={() => focusGroup(workspace.id, group.id)}
    >
      <IdeTabBar
        orgSlug={orgSlug}
        workspace={workspace}
        group={group}
        focused={focused}
        onFocus={() => focusGroup(workspace.id, group.id)}
      />
      <div className="min-h-0 flex-1 border-t border-border bg-card">
        {activeTab && isObject ? (
          <ObjectDetailView orgSlug={orgSlug} workspace={workspace} tab={activeTab} />
        ) : activeTab && doc ? (
          isLoading ? (
            <div className="flex h-full items-center justify-center text-xs text-muted-foreground">Loading…</div>
          ) : isError ? (
            <div className="flex h-full items-center justify-center text-xs text-destructive">
              Failed to load file content.
            </div>
          ) : (
            <SqlEditor
              key={`${group.id}:${activeTab.id}`}
              tabId={activeTab.id}
              groupId={group.id}
              doc={doc}
              className="h-full"
              onCursorChange={focused ? onCursorChange : undefined}
            />
          )
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            No editor in this group
          </div>
        )}
      </div>
    </div>
  )
}
