import type { Connection, ObjectRef, Workspace } from '#/lib/api/types'
import type { EditorTab } from '../useIdeStore'

export type DiagramTarget =
  | { kind: 'namespace'; namespace: string }
  | { kind: 'object'; ref: ObjectRef }

/** Stable id for a diagram tab. Re-opening the same target focuses the existing
 *  tab instead of creating a duplicate. */
export function diagramTabId(connectionId: number, target: DiagramTarget): string {
  return target.kind === 'namespace'
    ? `diagram:${connectionId}:ns:${target.namespace}`
    : `diagram:${connectionId}:obj:${target.ref.namespace}:${target.ref.kind}:${target.ref.name}`
}

export function newDiagramTab(connection: Connection, workspace: Workspace, target: DiagramTarget): EditorTab {
  const title = target.kind === 'namespace' ? target.namespace : target.ref.name
  const subtitle = target.kind === 'namespace' ? 'schema' : target.ref.namespace
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
