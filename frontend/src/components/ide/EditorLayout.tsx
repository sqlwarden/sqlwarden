import { Fragment } from 'react'
import { ResizablePanel, ResizablePanelGroup, ResizableHandle } from '#/components/ui/resizable'
import type { Workspace } from '#/lib/api/types'
import { useIde } from './useIdeStore'
import type { LayoutNode } from './ideLayout'
import { EditorGroup } from './EditorGroup'

type EditorLayoutProps = {
  orgSlug: string
  workspace: Workspace
  node: LayoutNode
  onCursorChange?: (line: number, col: number, sel: number) => void
  /** True when this node is rendered inside a split (so groups show a focus ring). */
  withinSplit?: boolean
}

/** Recursively renders the editor layout tree: splits → resizable panel groups,
 *  groups → EditorGroup (tab bar + active editor). */
export function EditorLayout({ orgSlug, workspace, node, onCursorChange, withinSplit = false }: EditorLayoutProps) {
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])

  if (node.type === 'group') {
    return (
      <EditorGroup
        orgSlug={orgSlug}
        workspace={workspace}
        group={node}
        focused={node.id === activeGroupId}
        showFocus={withinSplit}
        onCursorChange={onCursorChange}
      />
    )
  }

  return (
    <ResizablePanelGroup
      orientation={node.orientation === 'row' ? 'horizontal' : 'vertical'}
      className="min-h-0 flex-1 overflow-hidden"
    >
      {node.children.map((child, i) => (
        <Fragment key={child.id}>
          {i > 0 && <ResizableHandle withHandle />}
          <ResizablePanel defaultSize={`${100 / node.children.length}%`} minSize="15%" className="overflow-hidden">
            <EditorLayout orgSlug={orgSlug} workspace={workspace} node={child} onCursorChange={onCursorChange} withinSplit />
          </ResizablePanel>
        </Fragment>
      ))}
    </ResizablePanelGroup>
  )
}
