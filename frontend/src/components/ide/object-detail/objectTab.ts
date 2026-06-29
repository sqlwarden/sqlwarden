import type { Connection, ObjectRef, Workspace } from '#/lib/api/types'
import type { EditorTab } from '../useIdeStore'

/** Stable id for an object detail tab. Re-opening the same object focuses the
 *  existing tab instead of creating a duplicate. */
export function objectTabId(connectionId: number, ref: ObjectRef): string {
  return `object:${connectionId}:${ref.namespace}:${ref.kind}:${ref.name}`
}

export function newObjectTab(connection: Connection, workspace: Workspace, ref: ObjectRef): EditorTab {
  return {
    id: objectTabId(connection.id, ref),
    workspaceId: workspace.id,
    title: ref.name,
    kind: 'object',
    subtitle: ref.namespace,
    connectionId: connection.id,
    driver: connection.driver,
    objectRef: ref,
    content: '',
  }
}
