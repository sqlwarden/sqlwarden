import type { Connection, ObjectRef, ScopePath, Workspace } from '#/lib/api/types'
import { scopeKey, scopeLabel } from '#/lib/api/scope'
import type { EditorTab } from '../useIdeStore'

export type DiagramTarget = { kind: 'scope'; scope: ScopePath } | { kind: 'object'; ref: ObjectRef }

/** Stable id for a diagram tab. Re-opening the same target focuses the existing
 *  tab instead of creating a duplicate. */
export function diagramTabId(connectionId: number, target: DiagramTarget): string {
  return target.kind === 'scope'
    ? `diagram:${connectionId}:scope:${scopeKey(target.scope)}`
    : `diagram:${connectionId}:obj:${scopeKey(target.ref.scope)}:${target.ref.kind}:${target.ref.name}`
}

export function newDiagramTab(
  connection: Connection,
  workspace: Workspace,
  target: DiagramTarget,
): EditorTab {
  const title = target.kind === 'scope' ? scopeLabel(target.scope) : target.ref.name
  const subtitle = target.kind === 'scope' ? 'scope' : scopeLabel(target.ref.scope)
  return {
    id: diagramTabId(connection.id, target),
    workspaceId: workspace.id,
    title,
    kind: 'diagram',
    subtitle,
    connectionId: connection.id,
    driver: connection.driver,
    diagramTarget: target,
    content: '',
  }
}
