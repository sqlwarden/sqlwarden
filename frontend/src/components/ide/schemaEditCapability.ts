import type { SchemaEditOperation, SchemaEditSpec } from '#/lib/api/types'

export type SchemaEditGate = { allowed: true } | { allowed: false; reason: string }

const ALLOWED: SchemaEditGate = { allowed: true }

const NOT_SUPPORTED: SchemaEditGate = {
  allowed: false,
  reason: 'This driver does not support this change.',
}

const NOT_CONNECTED: SchemaEditGate = {
  allowed: false,
  reason: 'Connect to the database to make this change.',
}

const NOT_PERMITTED: SchemaEditGate = {
  allowed: false,
  reason: 'You do not have permission to change this schema.',
}

function baseGate(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  operation: SchemaEditOperation,
): SchemaEditGate | null {
  if (!editor || !editor.operations.includes(operation)) return NOT_SUPPORTED
  if (!sessionId) return NOT_CONNECTED
  if (!canMutate) return NOT_PERMITTED
  return null
}

export function canCreateTable(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  scopeKind: string,
): SchemaEditGate {
  const gate = baseGate(editor, sessionId, canMutate, 'create_table')
  if (gate) return gate
  if (!editor!.creatable_table_scope_kinds.includes(scopeKind)) return NOT_SUPPORTED
  return ALLOWED
}

export function canDropScope(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  scopeKind: string,
): SchemaEditGate {
  const gate = baseGate(editor, sessionId, canMutate, 'drop_scope')
  if (gate) return gate
  if (!editor!.droppable_scope_kinds.includes(scopeKind)) return NOT_SUPPORTED
  return ALLOWED
}

export function canDropObject(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  objectKind: string,
): SchemaEditGate {
  const gate = baseGate(editor, sessionId, canMutate, 'drop_object')
  if (gate) return gate
  if (!editor!.droppable_object_kinds.includes(objectKind)) return NOT_SUPPORTED
  return ALLOWED
}

/** rename_column/drop_column/drop_index only accept table references; views
 *  (and other relational objects) show columns/indexes but cannot be edited. */
function tableOnlyGate(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  objectKind: string,
  operation: SchemaEditOperation,
): SchemaEditGate {
  const gate = baseGate(editor, sessionId, canMutate, operation)
  if (gate) return gate
  if (objectKind !== 'table') return NOT_SUPPORTED
  return ALLOWED
}

export function canRenameColumn(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  objectKind: string,
): SchemaEditGate {
  return tableOnlyGate(editor, sessionId, canMutate, objectKind, 'rename_column')
}

export function canDropColumn(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  objectKind: string,
): SchemaEditGate {
  return tableOnlyGate(editor, sessionId, canMutate, objectKind, 'drop_column')
}

export function canDropIndex(
  editor: SchemaEditSpec | undefined,
  sessionId: string | undefined,
  canMutate: boolean,
  objectKind: string,
): SchemaEditGate {
  return tableOnlyGate(editor, sessionId, canMutate, objectKind, 'drop_index')
}

/** Targets a cascade checkbox can apply to. drop_index has no dependents to
 *  cascade over, so it's excluded regardless of driver support. */
export type CascadeTarget = 'scope' | 'object' | 'column' | 'index'

/** Cascade only makes sense alongside drop operations whose target can have
 *  dependents, and only when the driver opted in — never assume support from
 *  the operation list alone. */
export function cascadeAvailable(
  editor: SchemaEditSpec | undefined,
  target: CascadeTarget,
): boolean {
  if (target === 'index') return false
  return editor?.supports_cascade === true
}
